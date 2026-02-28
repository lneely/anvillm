#!/bin/bash
# capabilities: beads, tasks
# description: Create a new bead
# Usage: create_bead.sh <mount> <title> [description] [parent-id] [--no-lint] [capability=low|standard|high]
set -euo pipefail

if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

if [ $# -lt 2 ]; then
    echo "usage: create_bead.sh <mount> <title> [description] [parent-id] [--no-lint] [capability=low|standard|high]" >&2
    exit 1
fi

MOUNT="$1"
TITLE="$2"
DESC="${3:-}"
PARENT="${4:-}"
NOLINT="${5:-}"
CAP="${6:-}"

printf "new '%s' '%s' %s %s %s\n" "$TITLE" "$DESC" "$PARENT" "$NOLINT" "$CAP" | 9p write agent/beads/$MOUNT/ctl
