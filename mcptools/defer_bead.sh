#!/bin/bash
# capabilities: beads
# description: Defer a bead, optionally until a specific time
# Usage: defer_bead.sh <mount> <bead-id> [until <RFC3339-time>]
set -euo pipefail

if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

if [ $# -lt 2 ]; then
    echo "usage: defer_bead.sh <mount> <bead-id> [until <RFC3339-time>]" >&2
    exit 1
fi

MOUNT="$1"
BEAD_ID="$2"
shift 2

if [ $# -ge 2 ] && [ "$1" = "until" ]; then
    echo "defer $BEAD_ID until $2" | 9p write anvillm/beads/$MOUNT/ctl
    echo "deferred $BEAD_ID until $2"
else
    echo "defer $BEAD_ID" | 9p write anvillm/beads/$MOUNT/ctl
    echo "deferred $BEAD_ID"
fi
