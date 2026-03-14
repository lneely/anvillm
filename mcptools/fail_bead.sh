#!/bin/bash
# capabilities: beads
# description: Fail a bead with reason
# Usage: fail_bead.sh <mount> <bead-id> --reason <reason>
set -euo pipefail

if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

if [ $# -lt 2 ]; then
    echo "usage: fail_bead.sh <mount> <bead-id> --reason <reason>" >&2
    exit 1
fi

MOUNT="$1"
BEAD_ID="$2"
shift 2

REASON=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --reason) REASON="$2"; shift 2 ;;
        *) echo "unknown argument: $1" >&2; exit 1 ;;
    esac
done

if [ -z "$REASON" ]; then
    echo "error: --reason is required" >&2
    exit 1
fi

printf "fail %s '%s'\n" "$BEAD_ID" "$REASON" | 9p write anvillm/beads/$MOUNT/ctl
echo "failed $BEAD_ID: $REASON"
