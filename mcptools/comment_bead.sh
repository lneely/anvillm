#!/bin/bash
# capabilities: beads
# description: Add a comment to a bead
# Usage: comment_bead.sh --mount <mount> --id <bead-id> --text <text>
set -euo pipefail


MOUNT=""
BEAD_ID=""
TEXT=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --mount) MOUNT="$2";   shift 2 ;;
        --id)    BEAD_ID="$2"; shift 2 ;;
        --text)  TEXT="$2";    shift 2 ;;
        *) echo "unknown argument: $1" >&2; exit 1 ;;
    esac
done

if [ -z "$MOUNT" ] || [ -z "$BEAD_ID" ] || [ -z "$TEXT" ]; then
    echo "usage: comment_bead.sh --mount <mount> --id <bead-id> --text <text>" >&2
    exit 1
fi

printf "comment %s '%s'\n" "$BEAD_ID" "$TEXT" | 9p write beads/$MOUNT/ctl
echo "commented on $BEAD_ID"
