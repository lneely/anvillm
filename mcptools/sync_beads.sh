#!/bin/bash
# capabilities: beads
# description: Sync a mounted beads project
# Usage: sync_beads.sh <mount>
set -euo pipefail

if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

if [ $# -lt 1 ]; then
    echo "usage: sync_beads.sh <mount>" >&2
    exit 1
fi

echo "sync" | 9p write "anvillm/beads/$1/ctl"
