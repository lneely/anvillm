#!/bin/bash
# capabilities: mcp
# description: Discover available MCP tools by capability and keyword
set -euo pipefail

# Usage: discover_tool <keyword>
# Returns: capability/tool-name pairs matching keyword

if [ $# -lt 1 ]; then
  echo "Usage: discover_tool <keyword>" >&2
  exit 1
fi

keyword="$1"

9p read agent/tools/help 2>/dev/null | grep -i "$keyword" | sort -u || echo "No tools found matching: $keyword"
