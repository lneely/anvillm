#!/bin/bash
# capabilities: mcp
# description: Discover available skills by intent and keyword
set -euo pipefail

# Usage: discover_skill <keyword>
# Returns: intent/skill-name pairs matching keyword

if [ $# -lt 1 ]; then
  echo "Usage: discover_skill <keyword>" >&2
  exit 1
fi

keyword="$1"

9p read agent/skills/help 2>/dev/null | grep -i "$keyword" | sort -u || echo "No skills found matching: $keyword"
