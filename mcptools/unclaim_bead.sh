#!/bin/bash
# capabilities: beads, tasks
# description: Unclaim a bead (reset to open)
# Usage: unclaim_bead.sh <bead-id>
set -euo pipefail

if cat /etc/passwd >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

if [ $# -lt 1 ]; then
    echo "usage: unclaim_bead.sh <bead-id>" >&2
    exit 1
fi

echo "unclaim $1" | 9p write agent/beads/ctl
