#!/bin/bash
# Parse and display message history for an agent or user
set -euo pipefail

AGENT_ID="${1:-}"
DATE_FILTER="${2:-}"
if [[ -z "$AGENT_ID" ]]; then
    echo "Usage: $0 <agent-id|user> [YYYYMMdd]" >&2
    exit 1
fi

MAIL_DIR="$HOME/.local/share/anvillm/mail/$AGENT_ID"

shopt -s nullglob
if [[ -n "$DATE_FILTER" ]]; then
    SENT="$MAIL_DIR/${DATE_FILTER}-sent.jsonl"
    RECV="$MAIL_DIR/${DATE_FILTER}-recv.jsonl"
    FILES=()
    [[ -f "$SENT" ]] && FILES+=("$SENT") || echo "Could not get sent messages for $DATE_FILTER" >&2
    [[ -f "$RECV" ]] && FILES+=("$RECV") || echo "Could not get received messages for $DATE_FILTER" >&2
else
    FILES=("$MAIL_DIR"/*-sent.jsonl "$MAIL_DIR"/*-recv.jsonl)
fi
shopt -u nullglob

if [[ ${#FILES[@]} -eq 0 ]]; then
    echo "No messages for $AGENT_ID"
    exit 0
fi

cat "${FILES[@]}" | jq -rs 'sort_by(.ts) | .[] |
"\(.ts | strftime("%Y-%m-%d %H:%M:%S")) [\(.data.type)] \(.data.from) → \(.data.to)
  Subject: \(.data.subject)
  \(.data.body | split("\n") | join("\n  "))
"'
