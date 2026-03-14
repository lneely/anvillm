#!/bin/bash
# capabilities: beads
# description: Sync a mounted beads project
# Usage: sync_beads.sh --mount <mount>
set -euo pipefail

if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

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

echo "sync" | 9p write "anvillm/beads/$MOUNT/ctl"
echo "synced $MOUNT"
