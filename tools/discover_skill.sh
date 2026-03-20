#!/bin/bash
# capabilities: discovery
# description: Discover available skills by keyword
set -euo pipefail

if [ $# -ne 1 ]; then
  echo "Usage: discover_skill <keyword>" >&2
  exit 1
fi

ANVILLM="${ANVILLM_9MOUNT:-$HOME/mnt/anvillm}"
keyword="$1"
results=""

for file in "$ANVILLM/skills"/*.md; do
  [ -f "$file" ] || continue
  name=$(basename "$file" .md)
  if grep -qi "$keyword" "$file"; then
    desc=$(grep -m1 '^description:' "$file" | sed 's/^description: *//')
    results="${results:+$results
}$name	$desc"
  fi
done

if [ -n "$results" ]; then
  echo "$results" | sort -u
else
  echo "No skills found matching: $keyword"
fi
