#!/bin/bash
# capabilities: beads
# description: Block until a bead matching --mount and --role is ready, then print its full JSON (including comments) and exit. Uses the anvillm/events stream — no polling.
# Usage: wait_for_bead.sh --mount <mount> [--role <role>]
set -euo pipefail

MOUNT=""
ROLE="${AGENT_ROLE:-developer}"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --mount) MOUNT="$2"; shift 2 ;;
        --role)  ROLE="$2";  shift 2 ;;
        *) echo "unknown argument: $1" >&2; exit 1 ;;
    esac
done

if [ -z "$MOUNT" ]; then
    echo "usage: wait_for_bead.sh --mount <mount> [--role <role>]" >&2
    exit 1
fi

EXPECTED_SOURCE="beads/$MOUNT"

while IFS= read -r line; do
    type=$(echo "$line" | jq -r '.type // empty' 2>/dev/null)
    [ "$type" = "BeadReady" ] || continue

    source=$(echo "$line" | jq -r '.source // empty' 2>/dev/null)
    [ "$source" = "$EXPECTED_SOURCE" ] || continue

    # Check role label matches (format: "role:<role>")
    bead_role=$(echo "$line" | jq -r '
        .data.labels // [] |
        map(select(startswith("role:"))) |
        first // "" |
        ltrimstr("role:")
    ' 2>/dev/null)
    # Default to "developer" if no role label
    [ -z "$bead_role" ] && bead_role="developer"
    [ "$bead_role" = "$ROLE" ] || continue

    # Emit full bead data and exit
    echo "$line" | jq '.data'
    exit 0
done < <(9p read anvillm/events)
