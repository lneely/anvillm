#!/bin/bash
# capabilities: beads
# description: Initialize beads project with issue prefix
# Usage: init_beads.sh [prefix]
set -euo pipefail

if cat /etc/passwd >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

echo "init ${1:-bd}" | 9p write agent/beads/ctl
