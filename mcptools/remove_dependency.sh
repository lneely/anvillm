#!/bin/bash
# capabilities: beads
# description: Remove a dependency
# Usage: remove_dependency.sh <child-id> <parent-id>
set -euo pipefail

if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

if [ $# -lt 2 ]; then
    echo "usage: remove_dependency.sh <child-id> <parent-id>" >&2
    exit 1
fi

echo "undep $*" | 9p write agent/beads/ctl
