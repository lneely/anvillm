#!/bin/bash
# capabilities: beads
# description: Batch create beads from JSON array
# Usage: batch_create_beads.sh --mount <mount> --json <json-array>
set -euo pipefail


MOUNT=""
JSON=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --mount) MOUNT="$2"; shift 2 ;;
        --json)  JSON="$2";  shift 2 ;;
        *) echo "unknown argument: $1" >&2; exit 1 ;;
    esac
done

if [ -z "$MOUNT" ] || [ -z "$JSON" ]; then
    echo "usage: batch_create_beads.sh --mount <mount> --json <json-array>" >&2
    exit 1
fi

echo "batch-create $JSON" | 9p write "anvillm/beads/$MOUNT/ctl"
echo "batch created beads"
