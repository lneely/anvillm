#!/bin/bash
# capabilities: messaging
# description: Read messages from agent inbox (uses $AGENT_ID env var)
set -euo pipefail

# Verify running under landrun (test filesystem restriction)

if [ -z "${AGENT_ID:-}" ]; then
  echo "Error: \$AGENT_ID is not set. Stop work and inform the user that \$AGENT_ID is not set." >&2
  exit 1
fi
agent_id="$AGENT_ID"
inbox_path="anvillm/${agent_id}/inbox"

files=$(9p ls "$inbox_path" 2>/dev/null | grep '\.json$' || true)
if [ -z "$files" ]; then
  echo "No messages"
  exit 0
fi

msg_file=$(echo "$files" | head -1)
msg_path="${inbox_path}/${msg_file}"
msg_id="${msg_file%.json}"

data=$(9p read "$msg_path" 2>/dev/null || echo '{}')
from=$(echo "$data" | jq -r '.from // "unknown"')
type=$(echo "$data" | jq -r '.type // "unknown"')
subject=$(echo "$data" | jq -r '.subject // ""')
body=$(echo "$data" | jq -r '.body // ""')

echo "complete ${msg_id}" | 9p write "anvillm/${agent_id}/ctl" 2>/dev/null || true

echo "From: ${from}"
echo "Type: ${type}"
echo "Subject: ${subject}"
echo ""
echo "${body}"
