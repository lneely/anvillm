#!/bin/bash
# capabilities: beads
# description: Block until a bead is ready on the given mount, then print its full JSON (including comments) and exit. Uses the anvillm/events stream — no polling. Filters by scope derived from cwd.
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

# Derive scope from cwd relative to mount's cwd
MOUNT_CWD=$(9p read "beads/$MOUNT/cwd" 2>/dev/null)
if [ -n "$MOUNT_CWD" ]; then
    REL_PATH="${PWD#"$MOUNT_CWD"}"
    REL_PATH="${REL_PATH#/}"
    MY_SCOPE="${REL_PATH%%/*}"
else
    MY_SCOPE=""
fi

EXPECTED_SOURCE="beads/$MOUNT"

exec 3< <(9p read anvillm/events 2>/dev/null)
EVENTS_PID=$!
trap 'kill $EVENTS_PID 2>/dev/null; exec 3<&-' EXIT

while IFS= read -r line <&3; do
    type=$(echo "$line" | jq -r '.type // empty' 2>/dev/null)
    [ "$type" = "BeadReady" ] || continue

    source=$(echo "$line" | jq -r '.source // empty' 2>/dev/null)
    [ "$source" = "$EXPECTED_SOURCE" ] || continue

    # Strict scope match: both must be equal (empty or same value)
    bead_scope=$(echo "$line" | jq -r '.data.scope // empty' 2>/dev/null)
    [ "$bead_scope" = "$MY_SCOPE" ] || continue

    # Emit full bead data and exit — bot decides whether to claim
    echo "$line" | jq '.data'
    exit 0
done
