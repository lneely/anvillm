#!/bin/bash
# capabilities: beads
# description: List all beads across all mounted projects (JSON array)
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

9p read "anvillm/beads/$MOUNT/list" 2>/dev/null | jq '[.[] | {id, priority, title, status}]' || echo "[]"
