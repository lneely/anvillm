#!/bin/bash
# capabilities: beads
# description: Mount a beads project
# Usage: mount_beads.sh <cwd>
# Mount name is the first segment of a random UUID (8 hex chars, 32-bit entropy).
set -euo pipefail

if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

if [ $# -lt 1 ]; then
    echo "usage: mount_beads.sh <cwd>" >&2
    exit 1
fi

CWD="$1"
MOUNT=$(uuidgen | cut -d- -f1)

echo "mount $CWD $MOUNT" | 9p write anvillm/beads/ctl
echo "$MOUNT"
