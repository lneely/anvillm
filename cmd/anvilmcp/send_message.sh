#!/bin/bash
# send_message - Send message to another agent or user
# Usage: send_message <from> <to> <type> <subject> <body>

from="$1"
to="$2"
type="$3"
subject="$4"
body="$5"

json=$(jq -n \
  --arg from "$from" \
  --arg to "$to" \
  --arg type "$type" \
  --arg subject "$subject" \
  --arg body "$body" \
  '{from: $from, to: $to, type: $type, subject: $subject, body: $body}')

echo "$json" | 9p write "agent/${from}/mail"
