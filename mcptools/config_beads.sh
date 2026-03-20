#!/bin/bash
# capabilities: beads
# description: Set a beads database configuration key
# Usage: config_beads.sh --mount <mount> --key <key> --value <value>
set -euo pipefail


MOUNT=""
KEY=""
VALUE=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --mount) MOUNT="$2"; shift 2 ;;
        --key)   KEY="$2";   shift 2 ;;
        --value) VALUE="$2"; shift 2 ;;
        *) echo "unknown argument: $1" >&2; exit 1 ;;
    esac
done

if [ -z "$MOUNT" ] || [ -z "$KEY" ]; then
    echo "usage: config_beads.sh --mount <mount> --key <key> --value <value>" >&2
    exit 1
fi

echo "config $KEY $VALUE" | 9p write beads/$MOUNT/ctl
echo "config $KEY = $VALUE"
