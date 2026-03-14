#!/bin/bash
# capabilities: beads
# description: Reopen a closed bead (reset to open)
# Usage: reopen_bead.sh <mount> <bead-id>
set -euo pipefail

if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

if [ $# -lt 2 ]; then
    echo "usage: reopen_bead.sh <mount> <bead-id>" >&2
    exit 1
fi

echo "reopen $2" | 9p write anvillm/beads/$1/ctl
echo "reopened $2"
