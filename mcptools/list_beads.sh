#!/bin/bash
# capabilities: beads
# description: List all open beads (JSON array). Filters by scope derived from cwd (strict match).
# Usage: list_beads.sh --mount <mount>
set -euo pipefail


MOUNT=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --mount) MOUNT="$2"; shift 2 ;;
        *) echo "unknown argument: $1" >&2; exit 1 ;;
    esac
done

if [ -z "$MOUNT" ]; then
    echo "usage: list_beads.sh --mount <mount>" >&2
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

# Strict match: scope must equal (both empty or both same value)
9p read "beads/$MOUNT/list" 2>/dev/null | jq --arg scope "$MY_SCOPE" '[.[] | select((.scope // "") == $scope) | {id, priority, title, status, scope}]' || echo "[]"
