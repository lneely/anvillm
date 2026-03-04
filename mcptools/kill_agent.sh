#!/bin/bash
# capabilities: agents
# description: Kill agent session(s) running in the given directory: <agent-id>
set -euo pipefail

if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

agent_id="${1:?Usage: kill_agent.sh <agent-id>}"

if echo "kill" | 9p write "agent/$agent_id/ctl" 2>/dev/null; then
  echo "Killed $agent_id"
else
  echo "No session found with id $agent_id" >&2
  exit 1
fi
