#!/bin/bash
# capabilities: messaging
# description: Send message to agent or user (FROM uses $AGENT_ID)
# Usage: send_message.sh --to <id|user> --type <type> --subject <subject> --body <body>
set -euo pipefail


if [ -z "${AGENT_ID:-}" ]; then
  echo "Error: \$AGENT_ID is not set. Stop work and inform the user that \$AGENT_ID is not set." >&2
  exit 1
fi
from="$AGENT_ID"

to=""
type=""
subject=""
body=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --to)      to="$2";      shift 2 ;;
        --type)    type="$2";    shift 2 ;;
        --subject) subject="$2"; shift 2 ;;
        --body)    body="$2";    shift 2 ;;
        *) echo "unknown argument: $1" >&2; exit 1 ;;
    esac
done

if [ -z "$to" ] || [ -z "$type" ] || [ -z "$subject" ] || [ -z "$body" ]; then
    echo "usage: send_message.sh --to <id|user> --type <type> --subject <subject> --body <body>" >&2
    exit 1
fi

# Validate recipient exists (allow "user" as a special recipient)
if [ "$to" != "user" ]; then
  if ! 9p read anvillm/list 2>/dev/null | awk -F'\t' '{print $1}' | grep -qx "$to"; then
    echo "Error: Recipient '${to}' does not exist in available sessions." >&2
    exit 1
  fi
fi

json=$(jq -n \
  --arg from "$from" \
  --arg to "$to" \
  --arg type "$type" \
  --arg subject "$subject" \
  --arg body "$body" \
  '{from: $from, to: $to, type: $type, subject: $subject, body: $body}')

echo "$json" | 9p write "anvillm/${from}/mail"
echo "sent: $type → $to"
