#!/bin/bash
# capabilities: beads
# description: Update a bead field
# Usage: update_bead.sh <bead-id> <field> <value>
set -euo pipefail

if cat /etc/passwd >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

if [ $# -lt 3 ]; then
    echo "usage: update_bead.sh <bead-id> <field> <value>" >&2
    exit 1
fi

echo "update $*" | 9p write agent/beads/ctl
