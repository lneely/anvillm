#!/bin/bash
# capabilities: beads
# description: Resume work on a bead after approval/review
# Usage: resume_work.sh <bead-id> [assignee]
set -euo pipefail

if cat /etc/passwd >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

if [ $# -lt 1 ]; then
    echo "usage: resume_work.sh <bead-id> [assignee]" >&2
    exit 1
fi

echo "resume $*" | 9p write agent/beads/ctl
