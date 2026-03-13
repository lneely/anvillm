#!/bin/bash
# capabilities: beads
# description: Remove a label from a bead
# Usage: unlabel_bead.sh <mount> <bead-id> <label>
set -euo pipefail

if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

if [ $# -lt 3 ]; then
    echo "usage: unlabel_bead.sh <mount> <bead-id> <label>" >&2
    exit 1
fi

echo "unlabel $2 $3" | 9p write anvillm/beads/$1/ctl
echo "unlabeled $2: $3"
