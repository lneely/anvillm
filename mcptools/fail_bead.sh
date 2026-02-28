#!/bin/bash
# capabilities: beads, tasks
# description: Fail a bead with reason
# Usage: fail_bead.sh <mount> <bead-id> <reason>
set -euo pipefail

if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

if [ $# -lt 3 ]; then
    echo "usage: fail_bead.sh <mount> <bead-id> <reason>" >&2
    exit 1
fi

MOUNT="$1"
BEAD_ID="$2"
shift 2
REASON="$*"

printf "fail %s '%s'\n" "$BEAD_ID" "$REASON" | 9p write agent/beads/$MOUNT/ctl
