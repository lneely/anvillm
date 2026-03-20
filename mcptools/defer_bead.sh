#!/bin/bash
# capabilities: beads
# description: Defer a bead, optionally until a specific time
# Usage: defer_bead.sh --mount <mount> --id <bead-id> [--until <RFC3339-time>]
set -euo pipefail


MOUNT=""
BEAD_ID=""
UNTIL=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --mount) MOUNT="$2";   shift 2 ;;
        --id)    BEAD_ID="$2"; shift 2 ;;
        --until) UNTIL="$2";   shift 2 ;;
        *) echo "unknown argument: $1" >&2; exit 1 ;;
    esac
done

if [ -z "$MOUNT" ] || [ -z "$BEAD_ID" ]; then
    echo "usage: defer_bead.sh --mount <mount> --id <bead-id> [--until <RFC3339-time>]" >&2
    exit 1
fi

if [ -n "$UNTIL" ]; then
    echo "defer $BEAD_ID until $UNTIL" | 9p write beads/$MOUNT/ctl
    echo "deferred $BEAD_ID until $UNTIL"
else
    echo "defer $BEAD_ID" | 9p write beads/$MOUNT/ctl
    echo "deferred $BEAD_ID"
fi
