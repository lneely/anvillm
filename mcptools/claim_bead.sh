#!/bin/bash
# capabilities: beads
# description: Claim a bead for work. Assignee is taken from $AGENT_ID.
# Usage: claim_bead.sh --mount <mount> --id <bead-id>
set -euo pipefail


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
    echo "usage: claim_bead.sh --mount <mount> --id <bead-id>" >&2
    exit 1
fi

if [ -z "${AGENT_ID:-}" ]; then
    echo "error: AGENT_ID is not set" >&2
    exit 1
fi

echo "claim $BEAD_ID $AGENT_ID" | 9p write beads/$MOUNT/ctl

assignee=$(9p read beads/$MOUNT/$BEAD_ID/json 2>/dev/null | jq -r '.assignee // empty')
if [ "$assignee" != "$AGENT_ID" ]; then
    echo "error: claim failed (bead is assigned to '$assignee')" >&2
    exit 1
fi
echo "claimed $BEAD_ID → $AGENT_ID"
