#!/bin/bash
# capabilities: beads
# description: List all beads across all mounted projects (JSON array)
# Usage: list_beads.sh <mount>
set -euo pipefail

if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

if [ $# -lt 1 ]; then
    echo "usage: list_beads.sh <mount>" >&2
    exit 1
fi

9p read "agent/beads/$1/list" 2>/dev/null || echo "[]"
