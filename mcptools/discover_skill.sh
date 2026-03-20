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

# Search each skill file for the keyword
while IFS= read -r file; do
  [ "${file%.md}" = "$file" ] && continue  # skip non-.md entries
  name="${file%.md}"
  content=$(9p read "anvillm/skills/$file")
  if echo "$content" | grep -qi "$keyword"; then
    desc=$(echo "$content" | grep -m1 '^description:' | sed 's/^description: *//')
    results="${results:+$results
}$name	$desc"
  fi
done < <(9p ls anvillm/skills)

if [ -n "$results" ]; then
  echo "$results" | sort -u
else
  echo "No skills found matching: $keyword"
fi
