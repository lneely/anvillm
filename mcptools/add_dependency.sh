#!/bin/bash
# capabilities: beads
# description: Add a dependency (child depends on parent)
# Usage: add_dependency.sh --mount <mount> --child <child-id> --parent <parent-id>
set -euo pipefail

if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

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
    echo "usage: add_dependency.sh --mount <mount> --child <child-id> --parent <parent-id>" >&2
    exit 1
fi

echo "dep $CHILD $PARENT" | 9p write anvillm/beads/$MOUNT/ctl
echo "$CHILD depends on $PARENT"
