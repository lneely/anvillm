#!/bin/bash
# Metrics dashboard for anvilmcp

LOG_DIR="$HOME/.local/state/anvilmcp"
EXEC_LOG="$LOG_DIR/executions.jsonl"
TOKEN_LOG="$LOG_DIR/tokens.jsonl"
SEC_LOG="$LOG_DIR/security.jsonl"

echo "=== AnvilMCP Metrics Dashboard ==="
echo

if [ ! -f "$EXEC_LOG" ]; then
    echo "No execution logs found at $EXEC_LOG"
    exit 0
fi

echo "--- Execution Statistics ---"
total=$(wc -l < "$EXEC_LOG")
success=$(jq -s '[.[] | select(.success == true)] | length' "$EXEC_LOG")
failed=$((total - success))
error_rate=$(awk "BEGIN {printf \"%.1f\", ($failed / $total) * 100}")

echo "Total executions: $total"
echo "Successful: $success"
echo "Failed: $failed"
echo "Error rate: $error_rate%"
echo

echo "--- Executions by Language ---"
jq -s 'group_by(.language) | map({language: .[0].language, count: length}) | .[]' "$EXEC_LOG" | jq -r '"\(.language): \(.count)"'
echo

echo "--- Average Duration ---"
avg_duration=$(jq -s '[.[] | .duration] | add / length / 1000000' "$EXEC_LOG")
echo "Average: ${avg_duration}ms"
echo

if [ -f "$TOKEN_LOG" ]; then
    echo "--- Token Savings ---"
    avg_reduction=$(jq -s '[.[] | .reduction_percent] | add / length' "$TOKEN_LOG")
    echo "Average token reduction: ${avg_reduction}%"
    echo
fi

if [ -f "$SEC_LOG" ]; then
    echo "--- Security Events ---"
    total_events=$(wc -l < "$SEC_LOG")
    echo "Total security events: $total_events"
    jq -s 'group_by(.event_type) | map({type: .[0].event_type, count: length}) | .[]' "$SEC_LOG" | jq -r '"\(.type): \(.count)"'
    echo
    echo "Recent security events:"
    jq -s '.[-5:] | .[] | {timestamp, event_type, details}' "$SEC_LOG" | jq -r '"\(.timestamp): [\(.event_type)] \(.details)"'
else
    echo "--- Security Events ---"
    echo "No security events logged"
fi
echo

echo "--- Recent Errors ---"
jq -s '[.[] | select(.success == false)] | .[-5:] | .[] | {timestamp, language, error}' "$EXEC_LOG" | jq -r '"\(.timestamp): [\(.language)] \(.error)"' | tail -5

