#!/bin/bash
# capabilities: beads, tasks
# description: Remove a label from a bead
# Usage: unlabel_bead.sh <bead-id> <label>
set -euo pipefail

if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

if [ $# -lt 2 ]; then
    echo "usage: unlabel_bead.sh <bead-id> <label>" >&2
    exit 1
fi

echo "unlabel $*" | 9p write agent/beads/list
