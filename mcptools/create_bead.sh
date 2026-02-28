#!/bin/bash
# capabilities: beads, tasks
# description: Create a new bead
# Usage: create_bead.sh <title> [description] [parent-id] [--no-lint] [capability=low|standard|high]
set -euo pipefail

if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

if [ $# -lt 1 ]; then
    echo "usage: create_bead.sh <title> [description] [parent-id] [--no-lint] [capability=low|standard|high]" >&2
    exit 1
fi

# Find the first mounted project
MOUNT=$(9p ls agent/beads | grep -v '^ctl$' | grep -v '^mtab$' | grep -v '^ready$' | head -1)
if [ -z "$MOUNT" ]; then
    echo "Error: No beads projects mounted" >&2
    exit 1
fi

echo "new \"$1\" \"${2:-}\" ${3:-} ${4:-} ${5:-}" | 9p write agent/beads/$MOUNT/ctl
