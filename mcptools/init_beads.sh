#!/bin/bash
# capabilities: beads
# description: Initialize beads project with issue prefix
# Usage: init_beads.sh <mount> [prefix]
set -euo pipefail

if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

if [ $# -lt 1 ]; then
    echo "usage: init_beads.sh <mount> [prefix]" >&2
    exit 1
fi

mount="$1"
prefix="${2:-bd}"

echo "init $prefix" | 9p write "anvillm/beads/$mount/ctl"
