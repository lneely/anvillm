#!/bin/bash
# capabilities: agents
# description: Create a new agent session
# Usage: create_session.sh --backend <backend> --cwd <cwd> [--sandbox <sandbox>] [--model <model>]
set -euo pipefail


BACKEND=""
CWD=""
SANDBOX=""
MODEL=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --backend) BACKEND="$2"; shift 2 ;;
        --cwd)     CWD="$2";     shift 2 ;;
        --sandbox) SANDBOX="$2"; shift 2 ;;
        --model)   MODEL="$2";   shift 2 ;;
        *) echo "unknown argument: $1" >&2; exit 1 ;;
    esac
done

if [ -z "$BACKEND" ] || [ -z "$CWD" ]; then
    echo "usage: create_session.sh --backend <backend> --cwd <cwd> [--sandbox <sandbox>] [--model <model>]" >&2
    exit 1
fi

CMD="new $BACKEND $CWD"
[ -n "$SANDBOX" ] && CMD="$CMD sandbox=$SANDBOX"
[ -n "$MODEL" ]   && CMD="$CMD model=$MODEL"

echo "$CMD" | 9p write anvillm/ctl
echo "created session: $BACKEND $CWD"
