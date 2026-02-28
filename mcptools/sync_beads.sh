#!/bin/bash
# capabilities: beads, tasks
# description: Sync a mounted beads project
# Usage: sync_beads.sh <name>
set -euo pipefail

if cat /etc/passwd >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

if [ $# -lt 1 ]; then
    echo "usage: sync_beads.sh <name>" >&2
    exit 1
fi

echo "sync $1" | 9p write agent/beads/ctl
