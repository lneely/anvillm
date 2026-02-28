#!/bin/bash
# capabilities: beads, tasks
# description: List all beads across all mounted projects (JSON array)
set -euo pipefail

if cat /etc/passwd >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

9p read agent/beads/list
