#!/bin/bash
# capabilities: beads
# description: Resume work on a bead after approval/review
# Usage: resume_work.sh <mount> <bead-id> [assignee]
set -euo pipefail

if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

if [ $# -lt 2 ]; then
    echo "usage: resume_work.sh <mount> <bead-id> [assignee]" >&2
    exit 1
fi

MOUNT="$1"
BEAD_ID="$2"
ASSIGNEE="${3:-}"

if [ -n "$ASSIGNEE" ]; then
    echo "resume $BEAD_ID $ASSIGNEE" | 9p write anvillm/beads/$MOUNT/ctl
else
    echo "resume $BEAD_ID" | 9p write anvillm/beads/$MOUNT/ctl
fi
