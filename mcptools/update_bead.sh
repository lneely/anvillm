#!/bin/bash
# capabilities: beads
# description: Update a bead field directly
# Usage: update_bead.sh <mount> <bead-id> --field <field> --value <value>
set -euo pipefail

if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

if [ $# -lt 3 ]; then
    echo "usage: update_bead.sh <mount> <bead-id> --field <field> --value <value>" >&2
    exit 1
fi

MOUNT="$1"
BEAD_ID="$2"
shift 2

FIELD=""
VALUE=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --field) FIELD="$2"; shift 2 ;;
        --value) VALUE="$2"; shift 2 ;;
        *) echo "unknown argument: $1" >&2; exit 1 ;;
    esac
done

if [ -z "$FIELD" ]; then
    echo "error: --field is required" >&2
    exit 1
fi

printf "update %s %s '%s'\n" "$BEAD_ID" "$FIELD" "$VALUE" | 9p write anvillm/beads/$MOUNT/ctl
echo "updated $BEAD_ID.$FIELD: $VALUE"
