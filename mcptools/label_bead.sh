#!/bin/bash
# capabilities: beads
# description: Add a label to a bead
# Usage: label_bead.sh <mount> <bead-id> <label>
set -euo pipefail

if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

if [ $# -lt 3 ]; then
    echo "usage: label_bead.sh <mount> <bead-id> <label>" >&2
    exit 1
fi

echo "label $2 $3" | 9p write anvillm/beads/$1/ctl
echo "labeled $2: $3"
