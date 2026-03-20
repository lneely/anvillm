#!/bin/bash
# capabilities: agents
# description: Control a session (stop|restart|kill|refresh)
# Usage: control_session.sh --session-id <session-id> --command <stop|restart|kill|refresh>
set -euo pipefail


SESSION_ID=""
COMMAND=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --session-id) SESSION_ID="$2"; shift 2 ;;
        --command)    COMMAND="$2";    shift 2 ;;
        *) echo "unknown argument: $1" >&2; exit 1 ;;
    esac
done

if [ -z "$SESSION_ID" ] || [ -z "$COMMAND" ]; then
    echo "usage: control_session.sh --session-id <session-id> --command <stop|restart|kill|refresh>" >&2
    exit 1
fi

echo "$COMMAND" | 9p write "anvillm/$SESSION_ID/ctl"
echo "$COMMAND: $SESSION_ID"
