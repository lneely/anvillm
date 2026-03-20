#!/bin/bash
# capabilities: discovery
# description: List all available roles (JSON array)
set -euo pipefail

ANVILLM="${ANVILLM_9MOUNT:-$HOME/mnt/anvillm}"

if [ ! -d "$ANVILLM/roles" ]; then
  echo "[]"
  exit 0
fi

for focus_dir in "$ANVILLM/roles"/*/; do
  [ -d "$focus_dir" ] || continue
  focus_area=$(basename "$focus_dir")
  [ "$focus_area" = "help" ] && continue
  for role_file in "$focus_dir"*.md; do
    [ -f "$role_file" ] || continue
    role_name=$(basename "$role_file" .md)
    desc=$(awk '/^---$/,/^---$/ {if (/^description:/) {sub(/^description: */, ""); print; exit}}' "$role_file")
    printf '%s\t%s\t%s\n' "$focus_area" "$role_name" "$desc"
  done
done | jq -Rs '
  split("\n") | map(select(length > 0)) | map(
    split("\t") | {focus_area: .[0], name: .[1], description: .[2]}
  )
'
