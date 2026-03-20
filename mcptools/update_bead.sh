#!/bin/bash
# capabilities: beads
# description: Update a bead field directly
# Usage: update_bead.sh --mount <mount> --id <bead-id> --field <field> --value <value>
set -euo pipefail


MOUNT=""
BEAD_ID=""
FIELD=""
VALUE=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --mount) MOUNT="$2";   shift 2 ;;
        --id)    BEAD_ID="$2"; shift 2 ;;
        --field) FIELD="$2";   shift 2 ;;
        --value) VALUE="$2";   shift 2 ;;
        *) echo "unknown argument: $1" >&2; exit 1 ;;
    esac
done

if [ -z "$MOUNT" ] || [ -z "$BEAD_ID" ] || [ -z "$FIELD" ]; then
    echo "usage: update_bead.sh --mount <mount> --id <bead-id> --field <field> --value <value>" >&2
    exit 1
fi

if [ "$FIELD" = "parent" ]; then
    # Reparent: create new bead under parent with same data, then delete original
    JSON=$(9p read "beads/$MOUNT/$BEAD_ID/json")
    TITLE=$(echo "$JSON" | jq -r '.title')
    DESC=$(echo "$JSON" | jq -r '.description // ""')
    STATUS=$(echo "$JSON" | jq -r '.status')
    
    # Create under new parent
    printf "new '%s' '%s' %s\n" "$TITLE" "$DESC" "$VALUE" | 9p write beads/$MOUNT/ctl
    
    # Delete original
    printf "delete %s\n" "$BEAD_ID" | 9p write beads/$MOUNT/ctl
    
    echo "reparented $BEAD_ID under $VALUE"
else
    printf "update %s %s '%s'\n" "$BEAD_ID" "$FIELD" "$VALUE" | 9p write beads/$MOUNT/ctl
    echo "updated $BEAD_ID.$FIELD: $VALUE"
fi
