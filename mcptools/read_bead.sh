#!/bin/bash
# capabilities: beads
# description: Read a bead property
# Usage: read_bead.sh --mount <mount> --id <bead-id> --property <property>
# Properties: json, status, title, description, assignee, comments, labels
set -euo pipefail


MOUNT=""
BEAD_ID=""
PROPERTY=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --mount)    MOUNT="$2";    shift 2 ;;
        --id)       BEAD_ID="$2";  shift 2 ;;
        --property) PROPERTY="$2"; shift 2 ;;
        *) echo "unknown argument: $1" >&2; exit 1 ;;
    esac
done

if [ -z "$MOUNT" ] || [ -z "$BEAD_ID" ] || [ -z "$PROPERTY" ]; then
    echo "usage: read_bead.sh --mount <mount> --id <bead-id> --property <property>" >&2
    exit 1
fi

9p read "beads/$MOUNT/$BEAD_ID/$PROPERTY" 2>/dev/null || echo "Property not found: $PROPERTY"
