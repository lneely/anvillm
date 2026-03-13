#!/bin/bash
# capabilities: beads
# description: Mark a bead as pending approval
# Usage: mark_pending_approval.sh <mount> <bead-id> [assignee]
set -euo pipefail

if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

if [ $# -lt 2 ]; then
    echo "usage: mark_pending_approval.sh <mount> <bead-id> [assignee]" >&2
    exit 1
fi

MOUNT="$1"
BEAD_ID="$2"
ASSIGNEE="${3:-}"

if [ -n "$ASSIGNEE" ]; then
    echo "pending-approval $BEAD_ID $ASSIGNEE" | 9p write anvillm/beads/$MOUNT/ctl
else
    echo "pending-approval $BEAD_ID" | 9p write anvillm/beads/$MOUNT/ctl
fi
echo "pending-approval: $BEAD_ID${ASSIGNEE:+ → $ASSIGNEE}"
