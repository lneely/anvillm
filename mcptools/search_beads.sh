#!/bin/bash
# capabilities: beads
# description: Search beads by id, title, or description content
# Usage: search_beads.sh --mount <mount> --query <query>
set -euo pipefail

if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

MOUNT=""
QUERY=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --mount) MOUNT="$2"; shift 2 ;;
        --query) QUERY="$2"; shift 2 ;;
        *) echo "unknown argument: $1" >&2; exit 1 ;;
    esac
done

if [ -z "$MOUNT" ] || [ -z "$QUERY" ]; then
    echo "usage: search_beads.sh --mount <mount> --query <query>" >&2
    exit 1
fi

9p read "anvillm/beads/$MOUNT/search/$QUERY" 2>/dev/null | jq '[.[] | {id, title, status, match_in: (if .id | test("'"$QUERY"'"; "i") then "id" elif .title | test("'"$QUERY"'"; "i") then "title" else "description" end)}]' || echo "[]"
