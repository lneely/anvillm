#!/bin/bash
# Search message history for an agent or user by regex pattern.
# Usage: mail_search.sh --agent-id <agent-id|user> --pattern <regex> [--date YYYYMMdd]
set -euo pipefail

AGENT_ID=""
PATTERN=""
DATE=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --agent-id) AGENT_ID="$2"; shift 2 ;;
        --pattern)  PATTERN="$2";  shift 2 ;;
        --date)     DATE="$2";     shift 2 ;;
        *) echo "unknown argument: $1" >&2; exit 1 ;;
    esac
done

if [[ -z "$AGENT_ID" ]] || [[ -z "$PATTERN" ]]; then
    echo "usage: mail_search.sh --agent-id <agent-id|user> --pattern <regex> [--date YYYYMMdd]" >&2
    exit 1
fi

DATE_FLAG=""
[[ -n "$DATE" ]] && DATE_FLAG="--date $DATE"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
"$SCRIPT_DIR/mail_history.sh" --agent-id "$AGENT_ID" $DATE_FLAG | grep -E "$PATTERN"
