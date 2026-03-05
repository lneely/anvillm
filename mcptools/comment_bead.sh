#!/bin/bash
# capabilities: beads
# description: Add a comment to a bead
# Usage: comment_bead.sh <mount> <bead-id> <text>
set -euo pipefail

if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

if [ $# -lt 3 ]; then
    echo "usage: comment_bead.sh <mount> <bead-id> <text>" >&2
    exit 1
fi

MOUNT="$1"
BEAD_ID="$2"
shift 2
TEXT="$*"

printf "comment %s '%s'\n" "$BEAD_ID" "$TEXT" | 9p write agent/beads/$MOUNT/ctl
