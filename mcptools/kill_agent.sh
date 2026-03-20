#!/bin/bash
# capabilities: agents
# description: Kill agent session(s) running in the given directory
# Usage: kill_agent.sh --agent-id <agent-id>
set -euo pipefail


AGENT_ID=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --agent-id) AGENT_ID="$2"; shift 2 ;;
        *) echo "unknown argument: $1" >&2; exit 1 ;;
    esac
done

if [ -z "$AGENT_ID" ]; then
    echo "usage: kill_agent.sh --agent-id <agent-id>" >&2
    exit 1
fi

if echo "kill" | 9p write "anvillm/$AGENT_ID/ctl" 2>/dev/null; then
  echo "Killed $AGENT_ID"
else
  echo "No session found with id $AGENT_ID" >&2
  exit 1
fi
