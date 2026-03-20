#!/bin/bash
# capabilities: beads
# description: Reopen a closed bead (reset to open)
# Usage: reopen_bead.sh --mount <mount> --id <bead-id>
set -euo pipefail


MOUNT=""
BEAD_ID=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --mount) MOUNT="$2";   shift 2 ;;
        --id)    BEAD_ID="$2"; shift 2 ;;
        *) echo "unknown argument: $1" >&2; exit 1 ;;
    esac
done

if [ -z "$MOUNT" ] || [ -z "$BEAD_ID" ]; then
    echo "usage: reopen_bead.sh --mount <mount> --id <bead-id>" >&2
    exit 1
fi

echo "reopen $BEAD_ID" | 9p write beads/$MOUNT/ctl
echo "reopened $BEAD_ID"
