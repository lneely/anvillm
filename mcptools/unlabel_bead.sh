#!/bin/bash
# capabilities: beads
# description: Remove a label from a bead
# Usage: unlabel_bead.sh --mount <mount> --id <bead-id> --label <label>
set -euo pipefail

if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

MOUNT=""
BEAD_ID=""
LABEL=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --mount) MOUNT="$2";   shift 2 ;;
        --id)    BEAD_ID="$2"; shift 2 ;;
        --label) LABEL="$2";   shift 2 ;;
        *) echo "unknown argument: $1" >&2; exit 1 ;;
    esac
done

if [ -z "$MOUNT" ] || [ -z "$BEAD_ID" ] || [ -z "$LABEL" ]; then
    echo "usage: unlabel_bead.sh --mount <mount> --id <bead-id> --label <label>" >&2
    exit 1
fi

echo "unlabel $BEAD_ID $LABEL" | 9p write anvillm/beads/$MOUNT/ctl
echo "unlabeled $BEAD_ID: $LABEL"
