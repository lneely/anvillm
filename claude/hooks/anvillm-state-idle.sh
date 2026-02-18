#!/bin/sh
echo "$(date '+%Y-%m-%d %H:%M:%S') - Hook fired: Stop" >> /tmp/claude-hooks.log
cat > /dev/null

if [ -z "$AGENT_ID" ]; then
	echo "$(date '+%Y-%m-%d %H:%M:%S') - ERROR: AGENT_ID not set" >> /tmp/claude-hooks.log
	exit 0
fi

echo "$(date '+%Y-%m-%d %H:%M:%S') - Using AGENT_ID: $AGENT_ID" >> /tmp/claude-hooks.log

if echo idle | 9p write agent/$AGENT_ID/state 2>>/tmp/claude-hooks.log; then
	echo "$(date '+%Y-%m-%d %H:%M:%S') - SUCCESS: Wrote 'idle' to agent/$AGENT_ID/state" >> /tmp/claude-hooks.log
	
	# Check for pending messages in inbox
	if [ -n "$(9p ls agent/$AGENT_ID/inbox/ 2>/dev/null)" ]; then
		echo "$(date '+%Y-%m-%d %H:%M:%S') - INBOX: Pending messages detected" >> /tmp/claude-hooks.log
		echo "[INBOX] You have pending messages. Check with: 9p-read-inbox"
	fi
else
	echo "$(date '+%Y-%m-%d %H:%M:%S') - ERROR: Failed to write to agent/$AGENT_ID/state" >> /tmp/claude-hooks.log
fi
