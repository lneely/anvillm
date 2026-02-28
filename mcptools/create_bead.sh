#!/bin/bash
# capabilities: beads
# description: Create a new bead
# Usage: create_bead.sh <title> [description] [parent-id] [--no-lint] [capability=low|standard|high]
set -euo pipefail

if cat /etc/passwd >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

if [ $# -lt 1 ]; then
    echo "usage: create_bead.sh <title> [description] [parent-id] [--no-lint] [capability=low|standard|high]" >&2
    exit 1
fi

echo "new $*" | 9p write agent/beads/ctl
