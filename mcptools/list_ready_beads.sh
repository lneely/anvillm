#!/bin/bash
# capabilities: beads
# description: List ready/claimable beads (JSON array)
# Usage: list_ready_beads.sh <mount>
set -euo pipefail

if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

if [ $# -lt 1 ]; then
    echo "usage: list_ready_beads.sh <mount>" >&2
    exit 1
fi

9p read "anvillm/beads/$1/ready" 2>/dev/null | jq '[.[] | {id, priority, title, status}]' || echo "[]"
