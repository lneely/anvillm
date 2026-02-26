#!/bin/bash
# capabilities: skills
# description: List all available skills (JSON array)
set -euo pipefail

if [ -z "${ANVILLM_SKILLS_PATH:-}" ]; then
  echo "[]"
  exit 0
fi

for dir in "$ANVILLM_SKILLS_PATH"/*; do
  if [ -d "$dir" ] && [ -f "$dir/SKILL.md" ]; then
    name=$(basename "$dir")
    desc=$(grep -m1 '^description:' "$dir/SKILL.md" | sed 's/^description: *//' || true)
    printf '%s\t%s\n' "$name" "$desc"
  fi
done | jq -Rs '
  split("\n") | map(select(length > 0)) | map(
    split("\t") | {name: .[0], description: .[1]}
  )
'
