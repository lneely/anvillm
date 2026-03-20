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

MAIL_DIR="$HOME/.local/share/anvillm/mail/$AGENT_ID"

shopt -s nullglob
if [[ -n "$DATE" ]]; then
    SENT="$MAIL_DIR/${DATE}-sent.jsonl"
    RECV="$MAIL_DIR/${DATE}-recv.jsonl"
    FILES=()
    [[ -f "$SENT" ]] && FILES+=("$SENT")
    [[ -f "$RECV" ]] && FILES+=("$RECV")
else
    FILES=("$MAIL_DIR"/*-sent.jsonl "$MAIL_DIR"/*-recv.jsonl)
fi
shopt -u nullglob

if [[ ${#FILES[@]} -eq 0 ]]; then
    exit 0
fi

cat "${FILES[@]}" | grep -E "$PATTERN"
