#!/bin/bash
# capabilities: beads, tasks
# description: Fail a bead with reason
# Usage: fail_bead.sh <bead-id> <reason>
set -euo pipefail

if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

if [ $# -lt 2 ]; then
    echo "usage: fail_bead.sh <bead-id> <reason>" >&2
    exit 1
fi

echo "fail $*" | 9p write agent/beads/ctl
