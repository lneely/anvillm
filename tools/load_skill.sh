#!/bin/bash
# capabilities: discovery
# description: Load a skill's SKILL.md by name
set -euo pipefail

# Usage: load_skill <skill-name> [skill-name...]
#
# Accepts either bare names or paths (the last component is used):
#   anvillm-sessions
#   skills/agents/anvillm-sessions  (legacy path format; last component is used)

if [ $# -lt 1 ]; then
  echo "Usage: load_skill <skill-name> [skill-name...]" >&2
  exit 1
fi

for arg in "$@"; do
  name="${arg##*/}"
  9p read "anvillm/skills/${name}.md"
done
