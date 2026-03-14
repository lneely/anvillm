#!/bin/bash
# capabilities: beads
# description: Mark a bead as pending approval
# Usage: mark_pending_approval.sh <mount> <bead-id> [--assignee <id>]
set -euo pipefail

if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

if [ $# -lt 2 ]; then
    echo "usage: mark_pending_approval.sh <mount> <bead-id> [--assignee <id>]" >&2
    exit 1
fi

MOUNT="$1"
BEAD_ID="$2"
shift 2

ASSIGNEE=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --assignee) ASSIGNEE="$2"; shift 2 ;;
        *) echo "unknown argument: $1" >&2; exit 1 ;;
    esac
done

if [ -n "$ASSIGNEE" ]; then
    echo "pending-approval $BEAD_ID $ASSIGNEE" | 9p write anvillm/beads/$MOUNT/ctl
else
    echo "pending-approval $BEAD_ID" | 9p write anvillm/beads/$MOUNT/ctl
fi
echo "pending-approval: $BEAD_ID${ASSIGNEE:+ → $ASSIGNEE}"
