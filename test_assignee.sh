#!/bin/bash
set -e

# Test assignee persistence in a separate namespace
NAMESPACE="test-assignee-$$"
export ANVILLM_NAMESPACE="$NAMESPACE"

echo "Starting test in namespace: $NAMESPACE"

# Start anvilsrv in background
anvilsrv &
SRV_PID=$!
sleep 2

# Create test bead
echo "Creating test bead..."
BEAD_ID=$(echo "new 'Test assignee' 'Testing assignee field'" | 9p write agent/beads/ctl 2>&1 || true)
BEAD_ID=$(9p read agent/beads/list | jq -r '.[-1].id')
echo "Created bead: $BEAD_ID"

# Check initial state
echo "Initial state:"
9p read agent/beads/$BEAD_ID/json | jq '{id, status, assignee}'

# Claim the bead
echo -e "\nClaiming bead..."
echo "claim $BEAD_ID" | 9p write agent/beads/ctl

# Check after claim
echo "After claim:"
9p read agent/beads/$BEAD_ID/json | jq '{id, status, assignee}'

# Complete the bead
echo -e "\nCompleting bead..."
echo "complete $BEAD_ID" | 9p write agent/beads/ctl

# Check after complete
echo "After complete:"
9p read agent/beads/$BEAD_ID/json | jq '{id, status, assignee}'

# Cleanup
kill $SRV_PID 2>/dev/null || true
rm -rf ~/.anvillm/namespaces/$NAMESPACE

echo -e "\nTest completed successfully!"
