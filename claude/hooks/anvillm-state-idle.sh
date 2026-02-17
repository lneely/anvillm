#!/bin/sh
echo "$(date '+%Y-%m-%d %H:%M:%S') - Hook fired: Stop (PPID=$PPID)" >> /tmp/claude-hooks.log
cat > /dev/null
pid=$PPID
while [ $pid -gt 1 ]; do
	if ps -p $pid -o comm= | grep -q claude; then
		break
	fi
	pid=$(ps -p $pid -o ppid= | tr -d ' ')
done
if [ $pid -le 1 ]; then
	echo "$(date '+%Y-%m-%d %H:%M:%S') - ERROR: Claude PID not found" >> /tmp/claude-hooks.log
	exit 0
fi
echo "$(date '+%Y-%m-%d %H:%M:%S') - Found Claude PID: $pid" >> /tmp/claude-hooks.log
# Read session ID from filesystem directly (9p read doesn't work in sandbox)
ns_path="/tmp/ns.$USER.:0/anvillm-session-id-$pid"
if [ ! -f "$ns_path" ]; then
	ns_path="/tmp/ns.$USER/anvillm-session-id-$pid"
fi
session=$(cat "$ns_path" 2>/dev/null)
if [ -z "$session" ]; then
	echo "$(date '+%Y-%m-%d %H:%M:%S') - ERROR: Session ID not found for PID $pid" >> /tmp/claude-hooks.log
	exit 0
fi
echo "$(date '+%Y-%m-%d %H:%M:%S') - Found session ID: $session" >> /tmp/claude-hooks.log
if echo idle | 9p write agent/$session/state 2>>/tmp/claude-hooks.log; then
	echo "$(date '+%Y-%m-%d %H:%M:%S') - SUCCESS: Wrote 'idle' to agent/$session/state" >> /tmp/claude-hooks.log
	
	# Check for pending messages in inbox
	if [ -n "$(9p ls agent/$session/inbox/ 2>/dev/null)" ]; then
		echo "$(date '+%Y-%m-%d %H:%M:%S') - INBOX: Pending messages detected" >> /tmp/claude-hooks.log
		echo "[INBOX] You have pending messages. Check with: 9p-read-inbox"
	fi
else
	echo "$(date '+%Y-%m-%d %H:%M:%S') - ERROR: Failed to write to agent/$session/state" >> /tmp/claude-hooks.log
fi
