#!/bin/bash
# capabilities: beads
# description: Mount a beads project
# Usage: mount_beads.sh <cwd> [name]
set -euo pipefail

if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

if [ $# -lt 1 ]; then
    echo "usage: mount_beads.sh <cwd> [name]" >&2
    exit 1
fi

echo "mount $*" | 9p write agent/beads/ctl
