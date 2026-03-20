#!/bin/bash
# capabilities: beads
# description: Import GitHub issue into beads. Scope is auto-derived from cwd.
# Usage: github_to_beads.sh --mount <mount> --repo <owner/repo> --issue <issue-number>
set -euo pipefail

MOUNT=""
REPO=""
ISSUE=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --mount) MOUNT="$2"; shift 2 ;;
        --repo)  REPO="$2";  shift 2 ;;
        --issue) ISSUE="$2"; shift 2 ;;
        *) echo "unknown argument: $1" >&2; exit 1 ;;
    esac
done

if [ -z "$MOUNT" ] || [ -z "$REPO" ] || [ -z "$ISSUE" ]; then
    echo "usage: github_to_beads.sh --mount <mount> --repo <owner/repo> --issue <issue-number>" >&2
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
if 9p read "beads/$MOUNT/list" | jq -e ".[] | select(.title | contains(\"#$ISSUE\"))" >/dev/null 2>&1; then
    echo "Issue #$ISSUE already imported" >&2
    exit 0
fi

# Fetch issue data
data=$(gh issue view "$ISSUE" --repo "$REPO" --json number,title,body,state,labels 2>/dev/null)

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
echo "new \"$bead_title\" \"$body\" '' $SCOPE_ARG" | 9p write "beads/$MOUNT/ctl"

# Get created bead ID
bead_id=$(9p read "beads/$MOUNT/list" | jq -r ".[] | select(.title | contains(\"#$number\")) | .id" | head -1)

# Update issue type if not task
if [[ "$issue_type" != "task" ]]; then
    echo "update $bead_id issue_type $issue_type" | 9p write "beads/$MOUNT/ctl"
fi

# Update status if closed
if [[ "$status" == "closed" ]]; then
    echo "update $bead_id status closed" | 9p write "beads/$MOUNT/ctl"
fi

# Parse task list from body
if [[ -n "$body" ]]; then
    # Extract unchecked tasks: - [ ] Task name
    echo "$body" | grep -E '^\s*-\s+\[\s+\]' | sed -E 's/^\s*-\s+\[\s+\]\s*//' | while IFS= read -r task; do
        if [[ -n "$task" ]]; then
            echo "new \"$task\" \"\" $bead_id $SCOPE_ARG" | 9p write "beads/$MOUNT/ctl"
        fi
    done
fi

echo "$bead_id"
