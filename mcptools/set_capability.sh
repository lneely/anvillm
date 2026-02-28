#!/bin/bash
# capabilities: beads
# description: Set capability level on a bead (low|standard|high)
# Usage: set_capability.sh <bead-id> <level>
set -euo pipefail

if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

if [ $# -lt 2 ]; then
    echo "usage: set_capability.sh <bead-id> <level>" >&2
    exit 1
fi

echo "set-capability $*" | 9p write agent/beads/ctl
