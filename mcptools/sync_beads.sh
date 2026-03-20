#!/bin/bash
# capabilities: beads
# description: Sync a mounted beads project
# Usage: sync_beads.sh --mount <mount>
set -euo pipefail


MOUNT=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --mount) MOUNT="$2"; shift 2 ;;
        *) echo "unknown argument: $1" >&2; exit 1 ;;
    esac
done

if [ -z "$MOUNT" ]; then
    echo "usage: sync_beads.sh --mount <mount>" >&2
    exit 1
fi

echo "sync" | 9p write "beads/$MOUNT/ctl"
echo "synced $MOUNT"
