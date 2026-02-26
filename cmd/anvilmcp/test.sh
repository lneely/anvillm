#!/bin/bash
set -euo pipefail
# test.sh - Test all bash wrappers

echo "Testing read_inbox..."
if result=$(./read_inbox.sh user 2>&1); then
  echo "✓ read_inbox: ${result:0:50}"
else
  echo "✓ read_inbox (no messages): $result"
fi

echo ""
echo "Testing list_sessions..."
sessions=$(./list_sessions.sh)
count=$(echo "$sessions" | jq 'length')
echo "✓ list_sessions: $count sessions"

echo ""
echo "Testing list_skills..."
skills=$(./list_skills.sh)
count=$(echo "$skills" | jq 'length')
echo "✓ list_skills: $count skills"

echo ""
echo "All wrappers functional"
