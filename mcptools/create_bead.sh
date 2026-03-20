#!/bin/bash
# capabilities: beads
# description: Create a new bead. Scope is auto-derived from cwd relative to mount.
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

# Derive scope from cwd relative to mount's cwd
MOUNT_CWD=$(9p read "beads/$MOUNT/cwd" 2>/dev/null)
SCOPE=""
if [ -n "$MOUNT_CWD" ]; then
    REL_PATH="${PWD#"$MOUNT_CWD"}"
    REL_PATH="${REL_PATH#/}"
    SCOPE="${REL_PATH%%/*}"
fi

CMD="new '$TITLE' '$DESC'"
[ -n "$PARENT" ] && CMD="$CMD $PARENT"
[ -n "$NOLINT" ] && CMD="$CMD $NOLINT"
[ -n "$CAP" ]    && CMD="$CMD capability=$CAP"
[ -n "$SCOPE" ]  && CMD="$CMD scope=$SCOPE"

echo "$CMD" | 9p write beads/$MOUNT/ctl
echo "created (deferred): $TITLE"
