#!/bin/bash
# capabilities: beads, tasks
# description: Add a comment to a bead
# Usage: comment_bead.sh <bead-id> <text>
set -euo pipefail

if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

if [ $# -lt 2 ]; then
    echo "usage: comment_bead.sh <bead-id> <text>" >&2
    exit 1
fi

echo "comment $*" | 9p write agent/beads/ctl
