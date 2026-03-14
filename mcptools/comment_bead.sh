#!/bin/bash
# capabilities: beads
# description: Add a comment to a bead
# Usage: comment_bead.sh <mount> <bead-id> --text <text>
set -euo pipefail

if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

if [ $# -lt 2 ]; then
    echo "usage: comment_bead.sh <mount> <bead-id> --text <text>" >&2
    exit 1
fi

MOUNT="$1"
BEAD_ID="$2"
shift 2

TEXT=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --text) TEXT="$2"; shift 2 ;;
        *) echo "unknown argument: $1" >&2; exit 1 ;;
    esac
done

if [ -z "$TEXT" ]; then
    echo "error: --text is required" >&2
    exit 1
fi

printf "comment %s '%s'\n" "$BEAD_ID" "$TEXT" | 9p write anvillm/beads/$MOUNT/ctl
echo "commented on $BEAD_ID"
