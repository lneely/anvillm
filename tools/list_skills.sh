#!/bin/bash
# capabilities: discovery
# description: List all available skills (JSON array)
set -euo pipefail

ANVILLM="${ANVILLM_9MOUNT:-$HOME/mnt/anvillm}"

if [ ! -d "$ANVILLM/skills" ]; then
  echo "[]"
  exit 0
fi

for file in "$ANVILLM/skills"/*.md; do
  [ -f "$file" ] || continue
  name=$(basename "$file" .md)
  desc=$(grep -m1 '^description:' "$file" | sed 's/^description: *//' || true)
  printf '%s\t%s\n' "$name" "$desc"
done | jq -Rs '
  split("\n") | map(select(length > 0)) | map(
    split("\t") | {name: .[0], description: .[1]}
  )
'
