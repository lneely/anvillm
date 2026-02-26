#!/bin/bash
# capabilities: tasks, github
# description: Import GitHub issue into beads: github-to-beads OWNER/REPO ISSUE_NUMBER
set -euo pipefail

repo="${1:?Usage: github-to-beads OWNER/REPO ISSUE_NUMBER}"
issue="${2:?Usage: github-to-beads OWNER/REPO ISSUE_NUMBER}"

# Check if already imported
if 9p read agent/beads/list | jq -e ".[] | select(.title | contains(\"#$issue\"))" >/dev/null 2>&1; then
    echo "Issue #$issue already imported" >&2
    exit 0
fi

# Fetch issue data
data=$(gh issue view "$issue" --repo "$repo" --json number,title,body,state,labels 2>/dev/null)

number=$(echo "$data" | jq -r '.number')
title=$(echo "$data" | jq -r '.title')
body=$(echo "$data" | jq -r '.body // empty')
state=$(echo "$data" | jq -r '.state')
labels=$(echo "$data" | jq -r '.labels[].name' | tr '\n' ' ')

# Determine issue type from labels
issue_type="task"
if echo "$labels" | grep -qi "bug"; then
    issue_type="bug"
fi

# Determine status
status="open"
if [[ "$state" == "CLOSED" ]]; then
    status="closed"
fi

# Build title: #NUMBER: title
bead_title="#$number: $title"

# Create parent bead
echo "new \"$bead_title\" \"$body\"" | 9p write agent/beads/ctl

# Get created bead ID
bead_id=$(9p read agent/beads/list | jq -r ".[] | select(.title | contains(\"#$number\")) | .id" | head -1)

# Update issue type if not task
if [[ "$issue_type" != "task" ]]; then
    echo "update $bead_id issue_type $issue_type" | 9p write agent/beads/ctl
fi

# Update status if closed
if [[ "$status" == "closed" ]]; then
    echo "update $bead_id status closed" | 9p write agent/beads/ctl
fi

# Parse task list from body
if [[ -n "$body" ]]; then
    # Extract unchecked tasks: - [ ] Task name
    echo "$body" | grep -E '^\s*-\s+\[\s+\]' | sed -E 's/^\s*-\s+\[\s+\]\s*//' | while IFS= read -r task; do
        if [[ -n "$task" ]]; then
            echo "new \"$task\" \"\" $bead_id" | 9p write agent/beads/ctl
        fi
    done
fi

echo "$bead_id"
