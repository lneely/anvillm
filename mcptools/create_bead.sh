#!/bin/bash
# capabilities: beads
# description: Create a new bead
# Usage: create_bead.sh <mount> --title <title> [--desc <desc>] [--parent <id>] [--no-lint] [--capability low|standard|high]
set -euo pipefail

if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

if [ $# -lt 2 ]; then
    echo "usage: create_bead.sh <mount> --title <title> [--desc <desc>] [--parent <id>] [--no-lint] [--capability low|standard|high]" >&2
    exit 1
fi

MOUNT="$1"
shift

TITLE=""
DESC=""
PARENT=""
NOLINT=""
CAP=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --title)       TITLE="$2";      shift 2 ;;
        --desc)        DESC="$2";       shift 2 ;;
        --parent)      PARENT="$2";     shift 2 ;;
        --no-lint)     NOLINT="--no-lint"; shift ;;
        --capability)  CAP="$2";        shift 2 ;;
        *) echo "unknown argument: $1" >&2; exit 1 ;;
    esac
done

if [ -z "$TITLE" ]; then
    echo "error: --title is required" >&2
    exit 1
fi

CMD="new '$TITLE' '$DESC'"
[ -n "$PARENT" ] && CMD="$CMD $PARENT"
[ -n "$NOLINT" ] && CMD="$CMD $NOLINT"
[ -n "$CAP" ]    && CMD="$CMD $CAP"

echo "$CMD" | 9p write anvillm/beads/$MOUNT/ctl
echo "created (deferred): $TITLE"
