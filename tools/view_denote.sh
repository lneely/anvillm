#!/bin/bash
# capabilities: agent-kb
# description: View a denote document by identifier
# Usage: view_denote.sh --id <denote:identifier|identifier>
set -euo pipefail


ID=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --id) ID="$2"; shift 2 ;;
        *) echo "unknown argument: $1" >&2; exit 1 ;;
    esac
done

if [ -z "$ID" ]; then
    echo "usage: view_denote.sh --id <denote:identifier|identifier>" >&2
    exit 1
fi

ID="${ID#denote:}"
FILE=$(find "$HOME/doc" -type f -name "${ID}--*" | head -1)

if [ -z "$FILE" ]; then
    echo "Error: no denote document found for identifier $ID" >&2
    exit 1
fi

cat "$FILE"
