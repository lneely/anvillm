#!/bin/bash
# capabilities: beads
# description: Delete a bead
# Usage: delete_bead.sh <mount> <bead-id>
set -euo pipefail

if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

if [ $# -lt 2 ]; then
    echo "usage: delete_bead.sh <mount> <bead-id>" >&2
    exit 1
fi

echo "delete $2" | 9p write anvillm/beads/$1/ctl
