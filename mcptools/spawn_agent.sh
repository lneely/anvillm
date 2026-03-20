#!/bin/bash
# capabilities: agents
# description: Spawn a new agent session
# Usage: spawn_agent.sh --agent-id <agent-id> [--cwd <path>] [--prompt <initial-context>]
set -euo pipefail

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

backend=$(9p read "anvillm/$AGENT_ID/backend")
echo "new $backend $CWD" | 9p write anvillm/ctl

if [ -n "$PROMPT" ]; then
    session_id=$(9p read anvillm/list 2>/dev/null | awk -F'\t' -v cwd="$CWD" '$5 == cwd {print $1; exit}')
    if [ -n "$session_id" ]; then
        echo "$PROMPT" | 9p write "anvillm/$session_id/context"
    fi
fi
