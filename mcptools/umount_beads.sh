#!/bin/bash
# capabilities: beads
# description: Unmount a beads project
# Usage: umount_beads.sh <name>
set -euo pipefail

if cat /etc/passwd >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

if [ $# -lt 1 ]; then
    echo "usage: umount_beads.sh <name>" >&2
    exit 1
fi

echo "umount $1" | 9p write agent/beads/ctl
