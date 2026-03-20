#!/bin/bash
# capabilities: agents
# description: Spawn a new agent session
# Usage: spawn_agent.sh --agent-id <agent-id> [--cwd <path>] [--prompt <initial-context>]
set -euo pipefail

ANVILLM="${ANVILLM_9MOUNT:-$HOME/mnt/anvillm}"

AGENT_ID=""
CWD="$PWD"
PROMPT=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --agent-id) AGENT_ID="$2"; shift 2 ;;
        --cwd)      CWD="$2";      shift 2 ;;
        --prompt)   PROMPT="$2";   shift 2 ;;
        *) echo "unknown argument: $1" >&2; exit 1 ;;
    esac
done

if [ -z "$AGENT_ID" ]; then
    echo "usage: spawn_agent.sh --agent-id <agent-id> [--cwd <path>] [--prompt <text>]" >&2
    exit 1
fi

backend=$(cat "$ANVILLM/$AGENT_ID/backend")
echo "new $backend $CWD" > "$ANVILLM/ctl"

if [ -n "$PROMPT" ]; then
    session_id=$(cat "$ANVILLM/list" 2>/dev/null | awk -F'\t' -v cwd="$CWD" '$5 == cwd {print $1; exit}')
    if [ -n "$session_id" ]; then
        echo "$PROMPT" > "$ANVILLM/$session_id/context"
    fi
fi
