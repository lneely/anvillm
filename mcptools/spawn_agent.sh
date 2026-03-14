#!/bin/bash
# capabilities: agents
# description: Spawn a new agent session
# Usage: spawn_agent.sh <agent-id> [--cwd <path>] [--prompt <initial-context>]
set -euo pipefail

agent_id="${1:?Usage: spawn_agent.sh <agent-id> [--cwd <path>] [--prompt <text>]}"
shift

cwd="$PWD"
prompt=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --cwd)    cwd="$2";    shift 2 ;;
        --prompt) prompt="$2"; shift 2 ;;
        *) echo "unknown argument: $1" >&2; exit 1 ;;
    esac
done

backend=$(9p read "anvillm/$agent_id/backend")
echo "new $backend $cwd" | 9p write anvillm/ctl

if [ -n "$prompt" ]; then
    session_id=$(9p read anvillm/list 2>/dev/null | awk -F'\t' -v cwd="$cwd" '$5 == cwd {print $1; exit}')
    if [ -n "$session_id" ]; then
        echo "$prompt" | 9p write "anvillm/$session_id/context"
    fi
fi
