#!/bin/bash
# capabilities: beads
# description: Set a beads database configuration key
# Usage: config_beads.sh <mount> <key> <value>
set -euo pipefail

if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

if [ $# -lt 3 ]; then
    echo "usage: config_beads.sh <mount> <key> <value>" >&2
    exit 1
fi

echo "config $2 $3" | 9p write anvillm/beads/$1/ctl
echo "config $2 = $3"
