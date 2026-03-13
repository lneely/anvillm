#!/bin/bash
# capabilities: messaging
# description: Send message to agent or user: TO TYPE SUBJECT BODY (FROM uses $AGENT_ID)
set -euo pipefail

# Verify running under landrun (test filesystem restriction)
if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

if [ -z "${AGENT_ID:-}" ]; then
  echo "Error: \$AGENT_ID is not set. Stop work and inform the user that \$AGENT_ID is not set." >&2
  exit 1
fi
from="$AGENT_ID"

to="${1:?Usage: send_message <to> <type> <subject> <body>}"
type="${2:?Usage: send_message <to> <type> <subject> <body>}"
subject="${3:?Usage: send_message <to> <type> <subject> <body>}"
body="${4:?Usage: send_message <to> <type> <subject> <body>}"

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
