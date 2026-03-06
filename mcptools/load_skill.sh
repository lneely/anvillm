#!/bin/bash
# capabilities: discovery
# description: Load a skill's SKILL.md by name
set -euo pipefail

# Usage: load_skill <skill-name>
#
# Accepts either a bare name or a path (the last component is used):
#   anvillm-sessions
#   skills/agents/anvillm-sessions  (legacy path format; last component is used)

if [ $# -ne 1 ]; then
  echo "Usage: load_skill <skill-name>" >&2
  exit 1
fi

# Use the last path component as the skill name
name="${1##*/}"

9p read "agent/skills/${name}.md"
