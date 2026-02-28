#!/bin/bash
# capabilities: beads
# description: Set capability level on a bead (low|standard|high)
# Usage: set_capability.sh <mount> <bead-id> <level>
set -euo pipefail

if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

if [ $# -lt 3 ]; then
    echo "usage: set_capability.sh <mount> <bead-id> <level>" >&2
    exit 1
fi

echo "set-capability $2 $3" | 9p write agent/beads/$1/ctl
