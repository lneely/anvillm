#!/bin/bash
# capabilities: beads
# description: Import Jira ticket hierarchy into beads
# Usage: jira_to_beads.sh <mount> <ticket-key>
set -euo pipefail

mount="${1:?Usage: jira_to_beads.sh <mount> <ticket-key>}"
ticket="${2:?Usage: jira_to_beads.sh <mount> <ticket-key>}"

# Check if already imported
if 9p read "agent/beads/$mount/list" | jq -e ".[] | select(.title | contains(\"$ticket\"))" >/dev/null 2>&1; then
    echo "Ticket $ticket already imported" >&2
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
        echo "new \"$title\" \"$description\" $parent_bead" | 9p write "agent/beads/$mount/ctl"
    else
        echo "new \"$title\" \"$description\"" | 9p write "agent/beads/$mount/ctl"
    fi
    
    # Get created bead ID
    local bead_id
    bead_id=$(9p read "agent/beads/$mount/list" | jq -r ".[] | select(.title | contains(\"$key\")) | .id" | head -1)
    
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
root=$(find_root "$ticket")
echo "Importing from root: $root" >&2
create_bead "$root"
