#!/bin/bash
# Send a message to an agent, waiting for them to be idle first

if [ $# -lt 2 ]; then
    echo "Usage: $0 <agent-id> <message>"
    exit 1
fi

AGENT_ID="$1"
MESSAGE="$2"
MAX_ATTEMPTS=10
SLEEP_TIME=0.5

for i in $(seq 1 $MAX_ATTEMPTS); do
    STATE=$(9p read agent/$AGENT_ID/state 2>/dev/null)

    if [ "$STATE" = "idle" ]; then
        echo "$MESSAGE" | 9p write -l agent/$AGENT_ID/in
        exit $?
    fi

    if [ $i -lt $MAX_ATTEMPTS ]; then
        sleep $SLEEP_TIME
    fi
done

echo "Error: Agent $AGENT_ID did not become idle after $MAX_ATTEMPTS attempts" >&2
exit 1
