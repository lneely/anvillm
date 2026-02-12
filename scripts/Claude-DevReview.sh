#!/bin/bash
# Claude Developer-Reviewer Workflow Template
# Usage: ./scripts/Claude-DevReview.sh /path/to/project

set -e

if [ $# -ne 1 ]; then
    echo "Usage: $0 /path/to/project"
    exit 1
fi

PROJECT_PATH="$1"
PROJECT_NAME=$(basename "$PROJECT_PATH")

# Generate a random short identifier (Docker-style)
RANDOM_WORD=$(shuf -n1 -e alpha beta gamma delta zeta theta kappa sigma omega lambda)
RANDOM_NUM=$(printf "%02d" $((RANDOM % 100)))
ALIAS_NAME="${RANDOM_WORD}${RANDOM_NUM}"

echo "Setting up Developer-Reviewer workflow for: $PROJECT_NAME"
echo "Project path: $PROJECT_PATH"
echo "Session alias: $ALIAS_NAME"

# Get the 9p namespace
NAMESPACE=$(namespace)
if [ -z "$NAMESPACE" ]; then
    echo "Error: No namespace found. Is anvillm running?"
    exit 1
fi

AGENT_MOUNT="agent"

# Create developer session (non-blocking)
echo "Creating developer session..."
BEFORE=$(9p read $AGENT_MOUNT/list | awk '{print $1}' | sort)
echo "new claude $PROJECT_PATH" | 9p write $AGENT_MOUNT/ctl
AFTER=$(9p read $AGENT_MOUNT/list | awk '{print $1}' | sort)
DEV_ID=$(comm -13 <(echo "$BEFORE") <(echo "$AFTER") | head -1)
echo "Developer session ID: $DEV_ID"

# Create reviewer session (non-blocking)
echo "Creating reviewer session..."
BEFORE=$(9p read $AGENT_MOUNT/list | awk '{print $1}' | sort)
echo "new claude $PROJECT_PATH" | 9p write $AGENT_MOUNT/ctl
AFTER=$(9p read $AGENT_MOUNT/list | awk '{print $1}' | sort)
REVIEWER_ID=$(comm -13 <(echo "$BEFORE") <(echo "$AFTER") | head -1)
echo "Reviewer session ID: $REVIEWER_ID"

# Wait for both sessions to be ready
echo "Waiting for sessions to initialize..."
for i in {1..30}; do
    DEV_STATE=$(9p read $AGENT_MOUNT/$DEV_ID/state 2>/dev/null || echo "error")
    REV_STATE=$(9p read $AGENT_MOUNT/$REVIEWER_ID/state 2>/dev/null || echo "error")

    if [ "$DEV_STATE" = "idle" ] && [ "$REV_STATE" = "idle" ]; then
        echo "Both sessions ready!"
        break
    fi

    if [ $i -eq 30 ]; then
        echo "Error: Sessions failed to initialize (dev=$DEV_STATE, reviewer=$REV_STATE)"
        exit 1
    fi

    sleep 1
done

# Set aliases
echo "Setting aliases..."
echo "${ALIAS_NAME}-dev" | 9p write $AGENT_MOUNT/$DEV_ID/alias
echo "${ALIAS_NAME}-reviewer" | 9p write $AGENT_MOUNT/$REVIEWER_ID/alias

# Set developer context
echo "Configuring developer agent..."
cat > /tmp/dev-context-$$.txt <<'DEVCONTEXT'
# DEVELOPER AGENT ROLE

You are the developer agent in a two-agent workflow. Your peer is the reviewer agent.

## Your workflow:

1. When given a task, implement it fully
2. After implementation, stage all changes using git
3. Find your reviewer peer:
   - Run: 9p read agent/list | grep reviewer | grep PROJECT_NAME
   - Extract the session ID from the output
4. Send a review request to the reviewer:
   - Write to: agent/{reviewer-session-id}/in
   - Message: "Please review the staged changes"
5. Wait for the reviewer's response (they will send it to you via agent/{your-id}/in)
6. If you receive "LGTM" from the reviewer, you're done
7. If you receive suggested changes, implement them and repeat from step 2

## Communication Protocol:

- Use 9p commands to discover and communicate with your peer
- Read agent/list to find the reviewer agent
- Write to agent/{peer-id}/in to send messages
- You will receive messages from the reviewer in your own input stream

Remember: You are working collaboratively with the reviewer to ensure code quality.
DEVCONTEXT

# Replace PROJECT_NAME placeholder
sed "s/PROJECT_NAME/$ALIAS_NAME/g" /tmp/dev-context-$$.txt | 9p write $AGENT_MOUNT/$DEV_ID/context
rm /tmp/dev-context-$$.txt

# Set reviewer context
echo "Configuring reviewer agent..."
cat > /tmp/reviewer-context-$$.txt <<'REVIEWERCONTEXT'
# REVIEWER AGENT ROLE

You are the code reviewer agent in a two-agent workflow. Your peer is the developer agent.

## Your workflow:

1. When you receive a review request (via agent/{your-id}/in), review the staged changes
2. Use git commands to examine what's been staged:
   - git diff --cached
   - Look for bugs, code quality issues, security concerns
3. Find your developer peer:
   - Run: 9p read agent/list | grep dev | grep PROJECT_NAME
   - Extract the session ID from the output
4. Send feedback to the developer:
   - If the code is good: write "LGTM" to agent/{dev-session-id}/in
   - If changes needed: write specific, actionable feedback to agent/{dev-session-id}/in
5. Wait for the developer to implement changes and send another review request
6. Repeat until you can give "LGTM"

## Review Criteria:

- Code correctness
- Security (no SQL injection, XSS, command injection, etc.)
- Code style and readability
- Test coverage
- Error handling

## Communication Protocol:

- Use 9p commands to discover and communicate with your peer
- Read agent/list to find the developer agent
- Write to agent/{peer-id}/in to send messages
- You will receive review requests in your own input stream

Remember: Be thorough but constructive in your reviews. The goal is high-quality code.
REVIEWERCONTEXT

# Replace PROJECT_NAME placeholder
sed "s/PROJECT_NAME/$ALIAS_NAME/g" /tmp/reviewer-context-$$.txt | 9p write $AGENT_MOUNT/$REVIEWER_ID/context
rm /tmp/reviewer-context-$$.txt

echo ""
echo "✓ Developer-Reviewer workflow setup complete!"
echo ""
echo "Sessions created:"
echo "  Developer: $DEV_ID (alias: ${ALIAS_NAME}-dev)"
echo "  Reviewer:  $REVIEWER_ID (alias: ${ALIAS_NAME}-reviewer)"
echo ""
echo "To start development:"
echo "  1. Open the developer agent and give it a task"
echo "  2. The developer will implement, then automatically request review"
echo "  3. The reviewer will provide feedback"
echo "  4. The cycle continues until the reviewer gives LGTM"
echo ""
echo "To interact with the agents:"
echo "  - Open developer: echo open $DEV_ID | 9p write $AGENT_MOUNT/ctl"
echo "  - Open reviewer:  echo open $REVIEWER_ID | 9p write $AGENT_MOUNT/ctl"
echo ""
