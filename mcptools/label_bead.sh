#!/bin/bash
# capabilities: beads
# description: Add a label to a bead
# Usage: label_bead.sh <bead-id> <label>
set -euo pipefail

if cat /etc/passwd >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

if [ $# -lt 2 ]; then
    echo "usage: label_bead.sh <bead-id> <label>" >&2
    exit 1
fi

echo "label $*" | 9p write agent/beads/ctl
