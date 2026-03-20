#!/bin/bash
# capabilities: beads
# description: Import Jira ticket hierarchy into beads. Scope is auto-derived from cwd.
# Usage: jira_to_beads.sh --mount <mount> --ticket <ticket-key>
set -euo pipefail

MOUNT=""
TICKET=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --mount)  MOUNT="$2";  shift 2 ;;
        --ticket) TICKET="$2"; shift 2 ;;
        *) echo "unknown argument: $1" >&2; exit 1 ;;
    esac
done

if [ -z "$MOUNT" ] || [ -z "$TICKET" ]; then
    echo "usage: jira_to_beads.sh --mount <mount> --ticket <ticket-key>" >&2
    exit 1
fi

# Derive scope from cwd relative to mount's cwd
MOUNT_CWD=$(9p read "beads/$MOUNT/cwd" 2>/dev/null)
SCOPE=""
if [ -n "$MOUNT_CWD" ]; then
    REL_PATH="${PWD#"$MOUNT_CWD"}"
    REL_PATH="${REL_PATH#/}"
    SCOPE="${REL_PATH%%/*}"
fi
SCOPE_ARG=""
[ -n "$SCOPE" ] && SCOPE_ARG="scope=$SCOPE"

# Check if already imported
if 9p read "beads/$MOUNT/list" | jq -e ".[] | select(.title | contains(\"$TICKET\"))" >/dev/null 2>&1; then
    echo "Ticket $TICKET already imported" >&2
    exit 0
fi

# Find root ticket (walk up parent chain)
find_root() {
    local key="$1"
    local parent
    parent=$(jira issue view "$key" --raw 2>/dev/null | jq -r '.fields.parent.key // empty')
    if [[ -n "$parent" ]]; then
        find_root "$parent"
    else
        echo "$key"
    fi
}

# Create bead from ticket
create_bead() {
    local key="$1"
    local parent_bead="${2:-}"

    # Fetch ticket data
    local data
    data=$(jira issue view "$key" --raw 2>/dev/null)

    local summary
    summary=$(echo "$data" | jq -r '.fields.summary')

    local description
    description=$(echo "$data" | jq -r '.fields.description // empty')

    # Build title: KEY: summary
    local title="$key: $summary"

    # Create bead
    if [[ -n "$parent_bead" ]]; then
        echo "new \"$title\" \"$description\" $parent_bead $SCOPE_ARG" | 9p write "beads/$MOUNT/ctl"
    else
        echo "new \"$title\" \"$description\" '' $SCOPE_ARG" | 9p write "beads/$MOUNT/ctl"
    fi

    # Get created bead ID
    local bead_id
    bead_id=$(9p read "beads/$MOUNT/list" | jq -r ".[] | select(.title | contains(\"$key\")) | .id" | head -1)

    # Process children
    local children
    children=$(jira issue list --parent "$key" --plain --no-truncate 2>/dev/null | tail -n +2 | awk '{print $1}' || true)

    if [[ -n "$children" ]]; then
        while IFS= read -r child; do
            [[ -n "$child" ]] && create_bead "$child" "$bead_id"
        done <<< "$children"
    fi

    echo "$bead_id"
}

# Start import from root
root=$(find_root "$TICKET")
echo "Importing from root: $root" >&2
create_bead "$root"
