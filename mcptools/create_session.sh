#!/bin/bash
# capabilities: agents
# description: Create a new agent session
# Usage: create_session.sh <backend> <cwd> [role=<role>] [tasks=<task1,task2>] [model=<model>]
set -euo pipefail

if cat /etc/passwd >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

if [ $# -lt 2 ]; then
    echo "usage: create_session.sh <backend> <cwd> [role=<role>] [tasks=<task1,task2>] [model=<model>]" >&2
    exit 1
fi

echo "new $*" | 9p write agent/ctl
