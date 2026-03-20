#!/bin/bash
# capabilities: discovery
# description: Load a skill's SKILL.md by name
set -euo pipefail

# Usage: load_skill <skill-name> [skill-name...]

if [ $# -lt 1 ]; then
  echo "Usage: load_skill <skill-name> [skill-name...]" >&2
  exit 1
fi

ANVILLM="${ANVILLM_9MOUNT:-$HOME/mnt/anvillm}"

for arg in "$@"; do
  name="${arg##*/}"
  cat "$ANVILLM/skills/${name}.md"
done
