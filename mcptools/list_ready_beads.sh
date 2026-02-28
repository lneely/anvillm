#!/bin/bash
# capabilities: beads, tasks
# description: List ready/claimable beads (JSON array)
set -euo pipefail

if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

9p read agent/beads/ready 2>/dev/null || echo "[]"
