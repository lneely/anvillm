#!/bin/bash
# capabilities: beads, tasks
# description: Batch create beads from JSON array
# Usage: batch_create_beads.sh <json-array>
set -euo pipefail

if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

if [ $# -lt 1 ]; then
    echo "usage: batch_create_beads.sh <json-array>" >&2
    exit 1
fi

echo "batch-create $1" | 9p write agent/beads/list
