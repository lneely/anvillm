#!/bin/bash
# capabilities: beads
# description: Claim a bead for work
# Usage: claim_bead.sh --mount <mount> --id <bead-id> [--assignee <id>]
set -euo pipefail

if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

MOUNT=""
BEAD_ID=""
ASSIGNEE=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --mount)    MOUNT="$2";    shift 2 ;;
        --id)       BEAD_ID="$2";  shift 2 ;;
        --assignee) ASSIGNEE="$2"; shift 2 ;;
        *) echo "unknown argument: $1" >&2; exit 1 ;;
    esac
done

if [ -z "$MOUNT" ] || [ -z "$BEAD_ID" ]; then
    echo "usage: claim_bead.sh --mount <mount> --id <bead-id> [--assignee <id>]" >&2
    exit 1
fi

if [ -n "$ASSIGNEE" ]; then
    echo "claim $BEAD_ID $ASSIGNEE" | 9p write anvillm/beads/$MOUNT/ctl
else
    echo "claim $BEAD_ID" | 9p write anvillm/beads/$MOUNT/ctl
fi
echo "claimed $BEAD_ID${ASSIGNEE:+ → $ASSIGNEE}"
