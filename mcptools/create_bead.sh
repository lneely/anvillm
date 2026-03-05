#!/bin/bash
# capabilities: beads
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
shift
TITLE="$1"
shift
DESC="${1:-}"
[ $# -gt 0 ] && shift
PARENT="${1:-}"
[ $# -gt 0 ] && shift
NOLINT="${1:-}"
[ $# -gt 0 ] && shift
CAP="${1:-}"

CMD="new '$TITLE' '$DESC'"
[ -n "$PARENT" ] && CMD="$CMD $PARENT"
[ -n "$NOLINT" ] && CMD="$CMD $NOLINT"
[ -n "$CAP" ] && CMD="$CMD $CAP"

echo "$CMD" | 9p write agent/beads/$MOUNT/ctl
