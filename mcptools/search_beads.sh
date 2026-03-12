#!/bin/bash
# capabilities: beads
# description: Search beads by id, title, or description content
# Usage: search_beads.sh <mount> <query>
set -euo pipefail

if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

if [ $# -lt 2 ]; then
    echo "usage: search_beads.sh <mount> <query>" >&2
    exit 1
fi

mount="$1"
query="$2"

9p read "anvillm/beads/$mount/search/$query" 2>/dev/null | jq '[.[] | {id, title, status, match_in: (if .id | test("'"$query"'"; "i") then "id" elif .title | test("'"$query"'"; "i") then "title" else "description" end)}]' || echo "[]"
