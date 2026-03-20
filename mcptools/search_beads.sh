#!/bin/bash
# capabilities: beads
# description: Search beads by id, title, or description content. Filters by scope derived from cwd.
# Usage: search_beads.sh --mount <mount> --query <query>
#        search_beads.sh --mount <mount> --id <bead-id>
set -euo pipefail


MOUNT=""
QUERY=""
BEAD_ID=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --mount) MOUNT="$2"; shift 2 ;;
        --query) QUERY="$2"; shift 2 ;;
        --id)    BEAD_ID="$2"; shift 2 ;;
        *) echo "unknown argument: $1" >&2; exit 1 ;;
    esac
done

if [ -z "$MOUNT" ]; then
    echo "usage: search_beads.sh --mount <mount> --query <query>" >&2
    echo "       search_beads.sh --mount <mount> --id <bead-id>" >&2
    exit 1
fi

# Direct ID lookup
if [ -n "$BEAD_ID" ]; then
    9p read "beads/$MOUNT/$BEAD_ID/json" 2>/dev/null || echo "null"
    exit 0
fi

if [ -z "$QUERY" ]; then
    echo "usage: search_beads.sh --mount <mount> --query <query>" >&2
    echo "       search_beads.sh --mount <mount> --id <bead-id>" >&2
    exit 1
fi

# Derive scope from cwd relative to mount's cwd
MOUNT_CWD=$(9p read "beads/$MOUNT/cwd" 2>/dev/null)
MY_SCOPE=""
if [ -n "$MOUNT_CWD" ]; then
    REL_PATH="${PWD#"$MOUNT_CWD"}"
    REL_PATH="${REL_PATH#/}"
    MY_SCOPE="${REL_PATH%%/*}"
fi

9p read "beads/$MOUNT/search/$QUERY" 2>/dev/null | jq --arg scope "$MY_SCOPE" '[.[] | select((.scope // "") == $scope) | {id, title, status, scope, match_in: (if .id | test("'"$QUERY"'"; "i") then "id" elif .title | test("'"$QUERY"'"; "i") then "title" else "description" end)}]' || echo "[]"
