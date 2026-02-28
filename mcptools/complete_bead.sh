#!/bin/bash
# capabilities: beads, tasks
# description: Complete a bead
# Usage: complete_bead.sh <bead-id>
set -euo pipefail

if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

if [ $# -lt 1 ]; then
    echo "usage: complete_bead.sh <bead-id>" >&2
    exit 1
fi

echo "complete $1" | 9p write agent/beads/list
