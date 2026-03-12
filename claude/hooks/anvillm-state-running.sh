#!/bin/sh
echo "$(date '+%Y-%m-%d %H:%M:%S') - Hook fired: UserPromptSubmit" >> /tmp/claude-hooks.log

if [ -z "$AGENT_ID" ]; then
	echo "$(date '+%Y-%m-%d %H:%M:%S') - ERROR: AGENT_ID not set" >> /tmp/claude-hooks.log
	exit 0
fi

echo "$(date '+%Y-%m-%d %H:%M:%S') - Using AGENT_ID: $AGENT_ID" >> /tmp/claude-hooks.log

if echo running | 9p write anvillm/$AGENT_ID/state 2>>/tmp/claude-hooks.log; then
	echo "$(date '+%Y-%m-%d %H:%M:%S') - SUCCESS: Wrote 'running' to anvillm/$AGENT_ID/state" >> /tmp/claude-hooks.log
else
	echo "$(date '+%Y-%m-%d %H:%M:%S') - ERROR: Failed to write to anvillm/$AGENT_ID/state" >> /tmp/claude-hooks.log
fi
