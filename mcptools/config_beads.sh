#!/bin/bash
# capabilities: beads
# description: Set a beads database configuration key
# Usage: config_beads.sh --mount <mount> --key <key> --value <value>
set -euo pipefail

if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

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

echo "config $KEY $VALUE" | 9p write anvillm/beads/$MOUNT/ctl
echo "config $KEY = $VALUE"
