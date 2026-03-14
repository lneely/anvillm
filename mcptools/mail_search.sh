#!/bin/bash
# Search message history for an agent or user by regex pattern.
# Usage: mail_search.sh <agent-id|user> --pattern <regex> [--date YYYYMMdd]
set -euo pipefail

AGENT_ID="${1:-}"
if [[ -z "$AGENT_ID" ]]; then
    echo "Usage: $0 <agent-id|user> --pattern <regex> [--date YYYYMMdd]" >&2
    exit 1
fi
shift

PATTERN=""
DATE_FLAG=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --pattern) PATTERN="$2"; shift 2 ;;
        --date)    DATE_FLAG="--date $2"; shift 2 ;;
        *) echo "unknown argument: $1" >&2; exit 1 ;;
    esac
done

if [[ -z "$PATTERN" ]]; then
    echo "error: --pattern is required" >&2
    exit 1
fi

bash <(9p read anvillm/tools/mail_history.sh) "$AGENT_ID" $DATE_FLAG | grep -E "$PATTERN"
