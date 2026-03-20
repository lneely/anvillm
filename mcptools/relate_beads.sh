#!/bin/bash
# capabilities: beads
# description: Add a relates-to link between two beads
# Usage: relate_beads.sh --mount <mount> --bead1 <id> --bead2 <id>
set -euo pipefail


MOUNT=""
BEAD1=""
BEAD2=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --mount) MOUNT="$2"; shift 2 ;;
        --bead1) BEAD1="$2"; shift 2 ;;
        --bead2) BEAD2="$2"; shift 2 ;;
        *) echo "unknown argument: $1" >&2; exit 1 ;;
    esac
done

if [ -z "$MOUNT" ] || [ -z "$BEAD1" ] || [ -z "$BEAD2" ]; then
    echo "usage: relate_beads.sh --mount <mount> --bead1 <id> --bead2 <id>" >&2
    exit 1
fi

echo "relate $BEAD1 $BEAD2" | 9p write beads/$MOUNT/ctl
echo "related $BEAD1 ↔ $BEAD2"
