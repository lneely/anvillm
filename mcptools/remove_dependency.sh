#!/bin/bash
# capabilities: beads
# description: Remove a dependency
# Usage: remove_dependency.sh --mount <mount> --child <child-id> --parent <parent-id>
set -euo pipefail


MOUNT=""
CHILD=""
PARENT=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --mount)  MOUNT="$2";  shift 2 ;;
        --child)  CHILD="$2";  shift 2 ;;
        --parent) PARENT="$2"; shift 2 ;;
        *) echo "unknown argument: $1" >&2; exit 1 ;;
    esac
done

if [ -z "$MOUNT" ] || [ -z "$CHILD" ] || [ -z "$PARENT" ]; then
    echo "usage: remove_dependency.sh --mount <mount> --child <child-id> --parent <parent-id>" >&2
    exit 1
fi

echo "undep $CHILD $PARENT" | 9p write beads/$MOUNT/ctl
echo "$CHILD no longer depends on $PARENT"
