#!/bin/bash
# capabilities: mcp
# description: Discover available roles by focus area and keyword
set -euo pipefail

# Usage: discover_role <keyword>
# Returns: focus-area/role-name pairs matching keyword

if [ $# -lt 1 ]; then
  echo "Usage: discover_role <keyword>" >&2
  exit 1
fi

keyword="$1"

9p read agent/roles/help 2>/dev/null | grep -i "$keyword" | sort -u || echo "No roles found matching: $keyword"
