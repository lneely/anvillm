#!/bin/bash
# capabilities: messaging
# description: Send message to agent or user: FROM TO TYPE SUBJECT BODY
set -euo pipefail

from="${1:?Usage: send_message <from> <to> <type> <subject> <body>}"
to="${2:?Usage: send_message <from> <to> <type> <subject> <body>}"
type="${3:?Usage: send_message <from> <to> <type> <subject> <body>}"
subject="${4:?Usage: send_message <from> <to> <type> <subject> <body>}"
body="${5:?Usage: send_message <from> <to> <type> <subject> <body>}"

json=$(jq -n \
  --arg from "$from" \
  --arg to "$to" \
  --arg type "$type" \
  --arg subject "$subject" \
  --arg body "$body" \
  '{from: $from, to: $to, type: $type, subject: $subject, body: $body}')

echo "$json" | 9p write "agent/${from}/mail"
