#!/bin/bash
# capabilities: beads
# description: Block until a bead is ready on the given mount, then print its full JSON (including comments) and exit. Uses the anvillm/events stream — no polling.
# Usage: wait_for_bead.sh --mount <mount>
set -euo pipefail

MOUNT=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --mount) MOUNT="$2"; shift 2 ;;
        *) echo "unknown argument: $1" >&2; exit 1 ;;
    esac
done

if [ -z "$MOUNT" ]; then
    echo "usage: wait_for_bead.sh --mount <mount>" >&2
    exit 1
fi

EXPECTED_SOURCE="beads/$MOUNT"

while IFS= read -r line; do
    type=$(echo "$line" | jq -r '.type // empty' 2>/dev/null)
    [ "$type" = "BeadReady" ] || continue

    source=$(echo "$line" | jq -r '.source // empty' 2>/dev/null)
    [ "$source" = "$EXPECTED_SOURCE" ] || continue

    # Emit full bead data and exit — bot decides whether to claim
    echo "$line" | jq '.data'
    exit 0
done < <(9p read anvillm/events)
