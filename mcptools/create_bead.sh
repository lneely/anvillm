#!/bin/bash
# capabilities: beads
# description: Create a new bead
# Usage: create_bead.sh --mount <mount> --title <title> [--desc <desc>] [--parent <id>] [--no-lint] [--capability low|standard|high]
set -euo pipefail


MOUNT=""
TITLE=""
DESC=""
PARENT=""
NOLINT=""
CAP=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --mount)       MOUNT="$2";      shift 2 ;;
        --title)       TITLE="$2";      shift 2 ;;
        --desc)        DESC="$2";       shift 2 ;;
        --parent)      PARENT="$2";     shift 2 ;;
        --no-lint)     NOLINT="--no-lint"; shift ;;
        --capability)  CAP="$2";        shift 2 ;;
        *) echo "unknown argument: $1" >&2; exit 1 ;;
    esac
done

if [ -z "$MOUNT" ] || [ -z "$TITLE" ]; then
    echo "usage: create_bead.sh --mount <mount> --title <title> [--desc <desc>] [--parent <id>] [--no-lint] [--capability low|standard|high]" >&2
    exit 1
fi

CMD="new '$TITLE' '$DESC'"
[ -n "$PARENT" ] && CMD="$CMD $PARENT"
[ -n "$NOLINT" ] && CMD="$CMD $NOLINT"
[ -n "$CAP" ]    && CMD="$CMD $CAP"

echo "$CMD" | 9p write anvillm/beads/$MOUNT/ctl
echo "created (deferred): $TITLE"
