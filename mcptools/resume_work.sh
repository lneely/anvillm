#!/bin/bash
# capabilities: beads
# description: Resume work on a bead after approval/review
# Usage: resume_work.sh --mount <mount> --id <bead-id> [--assignee <id>]
set -euo pipefail


MOUNT=""
BEAD_ID=""
ASSIGNEE=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --mount)    MOUNT="$2";    shift 2 ;;
        --id)       BEAD_ID="$2";  shift 2 ;;
        --assignee) ASSIGNEE="$2"; shift 2 ;;
        *) echo "unknown argument: $1" >&2; exit 1 ;;
    esac
done

if [ -z "$MOUNT" ] || [ -z "$BEAD_ID" ]; then
    echo "usage: resume_work.sh --mount <mount> --id <bead-id> [--assignee <id>]" >&2
    exit 1
fi

if [ -n "$ASSIGNEE" ]; then
    echo "resume $BEAD_ID $ASSIGNEE" | 9p write anvillm/beads/$MOUNT/ctl
else
    echo "resume $BEAD_ID" | 9p write anvillm/beads/$MOUNT/ctl
fi
echo "resumed $BEAD_ID${ASSIGNEE:+ → $ASSIGNEE}"
