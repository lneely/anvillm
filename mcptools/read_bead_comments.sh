#!/bin/bash
# capabilities: beads
# description: Read comments for a bead
# Usage: read_comments.sh <mount> <bead-id>
set -euo pipefail

if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

if [ $# -lt 2 ]; then
    echo "usage: read_comments.sh <mount> <bead-id>" >&2
    exit 1
fi

9p read "agent/beads/$1/$2/comments" 2>/dev/null || echo "[]"
