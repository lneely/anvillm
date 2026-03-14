#!/bin/bash
# capabilities: discovery
# description: List all available roles (JSON array)
set -euo pipefail

# Verify running under landrun (test filesystem restriction)

9p ls anvillm/roles 2>/dev/null | grep -v '^help$' | while read focus_area; do
  9p ls "anvillm/roles/$focus_area" 2>/dev/null | while read role_file; do
    role_name="${role_file%.md}"
    desc=$(9p read "anvillm/roles/$focus_area/$role_file" 2>/dev/null | awk '/^---$/,/^---$/ {if (/^description:/) {sub(/^description: */, ""); print; exit}}')
    printf '%s\t%s\t%s\n' "$focus_area" "$role_name" "$desc"
  done
done | jq -Rs '
  split("\n") | map(select(length > 0)) | map(
    split("\t") | {focus_area: .[0], name: .[1], description: .[2]}
  )
'
