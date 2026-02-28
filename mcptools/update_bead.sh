#!/bin/bash
# capabilities: beads, tasks
# description: Update a bead field
# Usage: update_bead.sh <mount> <bead-id> <field> <value>
set -euo pipefail

if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

if [ $# -lt 4 ]; then
    echo "usage: update_bead.sh <mount> <bead-id> <field> <value>" >&2
    exit 1
fi

MOUNT="$1"
BEAD_ID="$2"
FIELD="$3"
shift 3
VALUE="$*"

printf "update %s %s '%s'\n" "$BEAD_ID" "$FIELD" "$VALUE" | 9p write agent/beads/$MOUNT/ctl
