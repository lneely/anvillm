#!/bin/bash
# capabilities: beads
# description: Add a relates-to link between two beads
# Usage: relate_beads.sh <mount> <bead-id-1> <bead-id-2>
set -euo pipefail

if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

if [ $# -lt 3 ]; then
    echo "usage: relate_beads.sh <mount> <bead-id-1> <bead-id-2>" >&2
    exit 1
fi

echo "relate $2 $3" | 9p write anvillm/beads/$1/ctl
echo "related $2 ↔ $3"
