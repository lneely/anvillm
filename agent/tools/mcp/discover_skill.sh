#!/bin/bash
# capabilities: mcp
# description: Discover available skills by intent and keyword
set -euo pipefail

# Usage: discover_skill <keyword>
# Returns: intent/skill-name pairs matching keyword
# Searches: path, description, name, and skill content

if [ $# -lt 1 ]; then
  echo "Usage: discover_skill <keyword>" >&2
  exit 1
fi

keyword="$1"
results=""

# Search help index (path and description)
if help_matches=$(9p read agent/skills/help 2>/dev/null | grep -i "$keyword"); then
  results="$help_matches"
fi

# Search skill content for keyword
while IFS= read -r intent; do
  [ "$intent" = "help" ] && continue
  while IFS= read -r skill; do
    path="$intent/$skill"
    # Skip if already in results
    echo "$results" | grep -q "^$path" && continue
    # Search skill content
    if 9p read "agent/skills/$path/SKILL.md" 2>/dev/null | grep -qi "$keyword"; then
      desc=$(9p read agent/skills/help 2>/dev/null | grep "^$path" | cut -f2)
      [ -n "$desc" ] && results="${results:+$results
}$path	$desc"
    fi
  done < <(9p ls "agent/skills/$intent" 2>/dev/null)
done < <(9p ls agent/skills 2>/dev/null)

if [ -n "$results" ]; then
  echo "$results" | sort -u
else
  echo "No skills found matching: $keyword"
fi
