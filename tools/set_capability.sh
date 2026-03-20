#!/bin/bash
# capabilities: beads
# description: Set capability level on a bead (low|standard|high)
# Usage: set_capability.sh --mount <mount> --id <bead-id> --level <low|standard|high>
set -euo pipefail


MOUNT=""
BEAD_ID=""
LEVEL=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --mount) MOUNT="$2";   shift 2 ;;
        --id)    BEAD_ID="$2"; shift 2 ;;
        --level) LEVEL="$2";   shift 2 ;;
        *) echo "unknown argument: $1" >&2; exit 1 ;;
    esac
done

if [ -z "$MOUNT" ] || [ -z "$BEAD_ID" ] || [ -z "$LEVEL" ]; then
    echo "usage: set_capability.sh --mount <mount> --id <bead-id> --level <low|standard|high>" >&2
    exit 1
fi

echo "set-capability $BEAD_ID $LEVEL" | 9p write beads/$MOUNT/ctl
echo "capability set: $BEAD_ID → $LEVEL"
