#!/bin/bash
# capabilities: beads
# description: Mark a bead as pending approval
# Usage: mark_pending_approval.sh <bead-id> [assignee]
set -euo pipefail

if cat /etc/passwd >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

if [ $# -lt 1 ]; then
    echo "usage: mark_pending_approval.sh <bead-id> [assignee]" >&2
    exit 1
fi

echo "pending-approval $*" | 9p write agent/beads/ctl
