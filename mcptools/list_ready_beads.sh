#!/bin/bash
# capabilities: beads
# description: List ready/claimable beads (JSON array)
# Usage: list_ready_beads.sh --mount <mount>
set -euo pipefail


MOUNT=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --mount) MOUNT="$2"; shift 2 ;;
        *) echo "unknown argument: $1" >&2; exit 1 ;;
    esac
done

if [ -z "$MOUNT" ]; then
    echo "usage: list_ready_beads.sh --mount <mount>" >&2
    exit 1
fi

9p read "anvillm/beads/$MOUNT/ready" 2>/dev/null | jq '[.[] | {id, priority, title, status}]' || echo "[]"
