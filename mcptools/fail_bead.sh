#!/bin/bash
# capabilities: beads
# description: Fail a bead with reason
# Usage: fail_bead.sh --mount <mount> --id <bead-id> --reason <reason>
set -euo pipefail


MOUNT=""
BEAD_ID=""
REASON=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --mount)  MOUNT="$2";   shift 2 ;;
        --id)     BEAD_ID="$2"; shift 2 ;;
        --reason) REASON="$2";  shift 2 ;;
        *) echo "unknown argument: $1" >&2; exit 1 ;;
    esac
done

if [ -z "$MOUNT" ] || [ -z "$BEAD_ID" ] || [ -z "$REASON" ]; then
    echo "usage: fail_bead.sh --mount <mount> --id <bead-id> --reason <reason>" >&2
    exit 1
fi

printf "fail %s '%s'\n" "$BEAD_ID" "$REASON" | 9p write beads/$MOUNT/ctl
echo "failed $BEAD_ID: $REASON"
