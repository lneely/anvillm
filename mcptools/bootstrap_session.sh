#!/bin/bash
# capabilities: beads, agents
# description: Build a bootstrap prompt for session continuity from a bead
# Usage: bootstrap_session.sh --mount <mount> --id <bead-id> [--git-lines <n>]
set -euo pipefail


MOUNT=""
BEAD_ID=""
GIT_LINES=30

while [[ $# -gt 0 ]]; do
    case "$1" in
        --mount)     MOUNT="$2";     shift 2 ;;
        --id)        BEAD_ID="$2";   shift 2 ;;
        --git-lines) GIT_LINES="$2"; shift 2 ;;
        *) echo "unknown argument: $1" >&2; exit 1 ;;
    esac
done

if [ -z "$MOUNT" ] || [ -z "$BEAD_ID" ]; then
    echo "usage: bootstrap_session.sh --mount <mount> --id <bead-id> [--git-lines <n>]" >&2
    exit 1
fi

# Get bead JSON
BEAD_JSON=$(9p read "anvillm/beads/$MOUNT/$BEAD_ID/json" 2>/dev/null) || {
    echo "error: bead $BEAD_ID not found in mount $MOUNT" >&2
    exit 1
}

# Extract fields
TITLE=$(echo "$BEAD_JSON" | jq -r '.title // ""')
STATUS=$(echo "$BEAD_JSON" | jq -r '.status // ""')
DESCRIPTION=$(echo "$BEAD_JSON" | jq -r '.description // ""')
ACCEPTANCE=$(echo "$BEAD_JSON" | jq -r '.acceptance_criteria // ""')
NOTES=$(echo "$BEAD_JSON" | jq -r '.notes // ""')
ASSIGNEE=$(echo "$BEAD_JSON" | jq -r '.assignee // ""')
COMMENT_COUNT=$(echo "$BEAD_JSON" | jq -r '.comment_count // 0')
PARENT_ID=$(echo "$BEAD_JSON" | jq -r '.parent_id // ""')

# Get comments if any
COMMENTS=""
if [ "$COMMENT_COUNT" -gt 0 ]; then
    COMMENTS=$(9p read "anvillm/beads/$MOUNT/$BEAD_ID/comments" 2>/dev/null \
        | jq -r '.[] | "[\(.created_at | split("T")[0])] \(.author // "agent"): \(.text)"' 2>/dev/null || true)
fi

# Get mount cwd from mtab
CWD=$(9p read anvillm/beads/mtab 2>/dev/null | awk -v m="$MOUNT" -F'\t' '$1 == m {print $2}')

# Get git log if we have a cwd
GIT_LOG=""
if [ -n "$CWD" ] && [ -d "$CWD/.git" ]; then
    GIT_LOG=$(git -C "$CWD" log --oneline -n "$GIT_LINES" 2>/dev/null || true)
fi

# Collect denote references from description, notes, and comments
ALL_TEXT="$DESCRIPTION $NOTES $COMMENTS"
DENOTE_IDS=$(echo "$ALL_TEXT" | grep -oE 'denote:[0-9]{8}T[0-9]{6}' | sed 's/denote://' | sort -u || true)

# Output bootstrap prompt
cat <<BOOTSTRAP
## Bootstrap Context — $BEAD_ID

### Task
**Title:** $TITLE
**Status:** $STATUS
**Mount:** $MOUNT${ASSIGNEE:+
**Assignee:** $ASSIGNEE}${PARENT_ID:+
**Parent:** $PARENT_ID}
${CWD:+**Project:** $CWD}

### Description
${DESCRIPTION:-"(none)"}

${ACCEPTANCE:+### Acceptance Criteria
$ACCEPTANCE

}${NOTES:+### Notes
$NOTES

}BOOTSTRAP

if [ -n "$COMMENTS" ]; then
    echo "### Comments"
    echo "$COMMENTS"
    echo ""
fi

if [ -n "$GIT_LOG" ]; then
    echo "### Recent Git History"
    echo "$GIT_LOG"
    echo ""
fi

# Include referenced denote documents
if [ -n "$DENOTE_IDS" ]; then
    echo "### Referenced Documents"
    while IFS= read -r did; do
        [ -z "$did" ] && continue
        FILE=$(find "$HOME/doc" -type f -name "${did}--*" 2>/dev/null | head -1)
        if [ -n "$FILE" ]; then
            echo ""
            echo "#### denote:$did ($(basename "$FILE"))"
            echo '```'
            cat "$FILE"
            echo '```'
        fi
    done <<< "$DENOTE_IDS"
fi
