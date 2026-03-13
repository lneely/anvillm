#!/bin/bash
# Search message history for an agent or user by regex pattern.
# Usage: mail_search.sh <agent-id|user> <pattern> [YYYYMMdd]
set -euo pipefail

AGENT_ID="${1:-}"
PATTERN="${2:-}"
DATE_FILTER="${3:-}"

if [[ -z "$AGENT_ID" || -z "$PATTERN" ]]; then
    echo "Usage: $0 <agent-id|user> <pattern> [YYYYMMdd]" >&2
    exit 1
fi

bash <(9p read anvillm/tools/mail_history.sh) "$AGENT_ID" "$DATE_FILTER" | grep -E "$PATTERN"
