#!/bin/bash
# capabilities: beads
# description: Batch create beads from JSON array. Scope is auto-derived from cwd.
# Usage: batch_create_beads.sh --mount <mount> --json <json-array>
set -euo pipefail


MOUNT=""
JSON=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --mount) MOUNT="$2"; shift 2 ;;
        --json)  JSON="$2";  shift 2 ;;
        *) echo "unknown argument: $1" >&2; exit 1 ;;
    esac
done

if [ -z "$MOUNT" ] || [ -z "$JSON" ]; then
    echo "usage: batch_create_beads.sh --mount <mount> --json <json-array>" >&2
    exit 1
fi

# Derive scope from cwd relative to mount's cwd
MOUNT_CWD=$(9p read "beads/$MOUNT/cwd" 2>/dev/null)
SCOPE=""
if [ -n "$MOUNT_CWD" ]; then
    REL_PATH="${PWD#"$MOUNT_CWD"}"
    REL_PATH="${REL_PATH#/}"
    SCOPE="${REL_PATH%%/*}"
fi

# Inject scope into each bead in the JSON array
if [ -n "$SCOPE" ]; then
    JSON=$(echo "$JSON" | jq --arg scope "$SCOPE" '[.[] | . + {scope: $scope}]')
fi

echo "batch-create $JSON" | 9p write "beads/$MOUNT/ctl"
echo "batch created beads"
