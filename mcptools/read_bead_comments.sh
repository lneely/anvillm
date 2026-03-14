#!/bin/bash
# capabilities: beads
# description: Read comments for a bead
# Usage: read_bead_comments.sh --mount <mount> --id <bead-id>
set -euo pipefail

if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

MOUNT=""
BEAD_ID=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --mount) MOUNT="$2";   shift 2 ;;
        --id)    BEAD_ID="$2"; shift 2 ;;
        *) echo "unknown argument: $1" >&2; exit 1 ;;
    esac
done

if [ -z "$MOUNT" ] || [ -z "$BEAD_ID" ]; then
    echo "usage: read_bead_comments.sh --mount <mount> --id <bead-id>" >&2
    exit 1
fi

9p read "anvillm/beads/$MOUNT/$BEAD_ID/comments" 2>/dev/null || echo "[]"
