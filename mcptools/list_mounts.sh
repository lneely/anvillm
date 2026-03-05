#!/bin/bash
# capabilities: beads
# description: List mounted beads projects
set -euo pipefail

if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

9p read agent/beads/mtab 2>/dev/null || echo "No mounted projects"
