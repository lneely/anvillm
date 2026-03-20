#!/bin/bash
# capabilities: beads
# description: Initialize beads project with issue prefix
# Usage: init_beads.sh --mount <mount> [--prefix <prefix>]
set -euo pipefail


MOUNT=""
PREFIX="bd"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --mount)  MOUNT="$2";  shift 2 ;;
        --prefix) PREFIX="$2"; shift 2 ;;
        *) echo "unknown argument: $1" >&2; exit 1 ;;
    esac
done

if [ -z "$MOUNT" ]; then
    echo "usage: init_beads.sh --mount <mount> [--prefix <prefix>]" >&2
    exit 1
fi

echo "init $PREFIX" | 9p write "beads/$MOUNT/ctl"
echo "initialized $MOUNT (prefix: $PREFIX)"
