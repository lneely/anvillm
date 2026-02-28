#!/bin/bash
# capabilities: agents
# description: Control a session (stop|restart|kill|refresh)
# Usage: control_session.sh <session-id> <command>
set -euo pipefail

if cat /etc/passwd >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

if [ $# -lt 2 ]; then
    echo "usage: control_session.sh <session-id> <command>" >&2
    exit 1
fi

echo "$2" | 9p write "agent/$1/ctl"
