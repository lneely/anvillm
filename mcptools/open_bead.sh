#!/bin/bash
# capabilities: beads
# description: Promote a deferred bead to open (ready for scheduling)
# Usage: open_bead.sh --mount <mount> --id <bead-id>
set -euo pipefail

if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

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
    echo "usage: open_bead.sh --mount <mount> --id <bead-id>" >&2
    exit 1
fi

echo "open $BEAD_ID" | 9p write anvillm/beads/$MOUNT/ctl
echo "opened $BEAD_ID"
