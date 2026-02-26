#!/bin/bash
# Test script to verify agents use execute_code pattern
# Usage: ./test-code-execution.sh

set -e

NAMESPACE=$(namespace)
if [ -z "$NAMESPACE" ]; then
    echo "Error: No namespace found. Is anvillm running?"
    exit 1
fi

AGENT_MOUNT="agent"

echo "=== Testing Code Execution Pattern Adoption ==="
echo ""

# Create test agent with Taskmaster template
echo "Creating test agent..."
echo "new kiro-cli ." | 9p write $AGENT_MOUNT/ctl
TEST_AGENT_ID=$(9p read $AGENT_MOUNT/list | head -1 | awk '{print $1}')
echo "test-agent" | 9p write $AGENT_MOUNT/$TEST_AGENT_ID/alias

# Load Taskmaster context
cat ./bot-templates/Taskmaster | grep -A 1000 "^cat <<EOF" | grep -B 1000 "^EOF$" | grep -v "^cat <<EOF" | grep -v "^EOF$" | 9p write $AGENT_MOUNT/$TEST_AGENT_ID/context

echo "Test agent created: $TEST_AGENT_ID"
echo ""

# Test query: Ask agent to find stale beads
echo "Test 1: Query for stale beads (should use execute_code)"
echo "Find all beads that haven't been updated in 30+ days and are still open. Show count by priority." | 9p write $AGENT_MOUNT/$TEST_AGENT_ID/prompt

# Wait for response
sleep 5

# Read response
echo ""
echo "Agent response:"
9p read $AGENT_MOUNT/$TEST_AGENT_ID/response
echo ""

# Check if execute_code was used
echo "Checking tool usage..."
TOOL_USAGE=$(9p read $AGENT_MOUNT/$TEST_AGENT_ID/stats 2>/dev/null || echo "")
if echo "$TOOL_USAGE" | grep -q "execute_code"; then
    echo "✓ Agent used execute_code"
else
    echo "✗ Agent did not use execute_code"
fi

echo ""
echo "Test 2: Batch operation (should use execute_code for loop)"
echo "Update all priority 1 beads to priority 2. Show progress." | 9p write $AGENT_MOUNT/$TEST_AGENT_ID/prompt

sleep 5

echo ""
echo "Agent response:"
9p read $AGENT_MOUNT/$TEST_AGENT_ID/response
echo ""

# Cleanup
echo "Cleaning up test agent..."
echo "stop $TEST_AGENT_ID" | 9p write $AGENT_MOUNT/ctl

echo ""
echo "=== Test Complete ==="
echo ""
echo "Manual verification steps:"
echo "1. Check if agent used execute_code instead of multiple beads.list calls"
echo "2. Verify filtering happened in sandbox (minimal context usage)"
echo "3. Compare token usage to baseline (should be 80-99% reduction)"
