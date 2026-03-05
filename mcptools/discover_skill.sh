#!/bin/bash
# capabilities: discovery
# description: Discover available skills by keyword
set -euo pipefail

if [ $# -ne 1 ]; then
  echo "Usage: discover_skill <keyword>" >&2
  exit 1
fi

keyword="$1"
results=""

# Search help index (path and description), resolve full path
if help_matches=$(9p read agent/skills/help | grep -i "$keyword"); then
  while IFS=$'\t' read -r skill desc; do
    for intent in $(9p ls agent/skills | grep -v '^help$'); do
      if 9p ls "agent/skills/$intent" | grep -qx "$skill"; then
        results="${results:+$results
}skills/$intent/$skill	$desc"
        break
      fi
    done
  done <<< "$help_matches"
fi

# Search skill content for keyword
while IFS= read -r intent; do
  [ "$intent" = "help" ] && continue
  while IFS= read -r skill; do
    path="skills/$intent/$skill"
    echo "$results" | grep -q "^$path" && continue
    if 9p read "agent/$path/SKILL.md" | grep -qi "$keyword"; then
      desc=$(9p read agent/skills/help | grep "^${skill}	" | cut -f2)
      [ -n "$desc" ] && results="${results:+$results
}$path	$desc"
    fi
  done < <(9p ls "agent/skills/$intent")
done < <(9p ls agent/skills)

if [ -n "$results" ]; then
  echo "$results" | sort -u
else
  echo "No skills found matching: $keyword"
fi
