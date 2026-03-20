#!/bin/bash
# Display message history for an agent or user.
# Usage: mail_history.sh --agent-id <agent-id|user> [--date YYYYMMdd]
set -euo pipefail

AGENT_ID=""
DATE_FILTER=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --agent-id) AGENT_ID="$2";    shift 2 ;;
        --date)     DATE_FILTER="$2"; shift 2 ;;
        *) echo "unknown argument: $1" >&2; exit 1 ;;
    esac
done

if [[ -z "$AGENT_ID" ]]; then
    echo "usage: mail_history.sh --agent-id <agent-id|user> [--date YYYYMMdd]" >&2
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

cat "${FILES[@]}" | jq -cs 'sort_by(.ts) | .[]'
