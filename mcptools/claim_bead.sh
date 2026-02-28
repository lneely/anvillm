#!/bin/bash
# capabilities: beads, tasks
# description: Claim a bead for work
# Usage: claim_bead.sh <bead-id> [assignee]
set -euo pipefail

if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

if [ $# -lt 1 ]; then
    echo "usage: claim_bead.sh <bead-id> [assignee]" >&2
    exit 1
fi

echo "claim $*" | 9p write agent/beads/list
