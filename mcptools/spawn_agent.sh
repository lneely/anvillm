#!/bin/bash
# capabilities: agents
# description: Spawn a new agent session (do not use execute_code): <agent-id> [cwd-path] [initial-context-prompt]
set -euo pipefail

agent_id="${1:?Usage: spawn_agent.sh <agent-id> [cwd-path] [initial-context-prompt]}"

if [ -d "${2:-}" ]; then
  cwd="$2"
  prompt="${3:-}"
else
  cwd="$PWD"
  prompt="${2:-}"
fi

backend=$(9p read "agent/$agent_id/backend")
echo "new $backend $cwd" | 9p write agent/ctl

if [ -n "$prompt" ]; then
  session_id=$(9p read agent/list 2>/dev/null | awk -F'\t' -v cwd="$cwd" '$5 == cwd {print $1; exit}')
  if [ -n "$session_id" ]; then
    echo "$prompt" | 9p write "agent/$session_id/context"
  fi
fi
