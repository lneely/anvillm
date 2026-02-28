#!/bin/bash
# capabilities: beads, tasks
# description: Read a bead property
# Usage: read_bead.sh <bead-id> <property>
set -euo pipefail

if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

if [ $# -lt 2 ]; then
    echo "usage: read_bead.sh <bead-id> <property>" >&2
    exit 1
fi

9p read "agent/beads/$1/$2" 2>/dev/null || echo "Property not found: $2"
