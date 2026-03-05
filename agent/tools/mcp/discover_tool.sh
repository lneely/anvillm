#!/bin/bash
# capabilities: mcp
# description: Discover available MCP tools by capability and keyword
set -euo pipefail

# Usage: discover_tool <keyword>
# Returns: capability/tool-name pairs matching keyword
# Searches: path, description, and tool content

if [ $# -lt 1 ]; then
  echo "Usage: discover_tool <keyword>" >&2
  exit 1
fi

keyword="$1"
results=""

# Search help index (path and description)
if help_matches=$(9p read agent/tools/help 2>/dev/null | grep -i "$keyword"); then
  results="$help_matches"
fi

# Search tool content for keyword
while IFS= read -r cap; do
  [ "$cap" = "help" ] && continue
  while IFS= read -r tool; do
    path="$cap/$tool"
    # Skip if already in results
    echo "$results" | grep -q "^$path" && continue
    # Search tool content
    if 9p read "agent/tools/$path" 2>/dev/null | grep -qi "$keyword"; then
      desc=$(9p read agent/tools/help 2>/dev/null | grep "^$path" | cut -f2)
      [ -n "$desc" ] && results="${results:+$results
}$path	$desc"
    fi
  done < <(9p ls "agent/tools/$cap" 2>/dev/null)
done < <(9p ls agent/tools 2>/dev/null)

if [ -n "$results" ]; then
  echo "$results" | sort -u
else
  echo "No tools found matching: $keyword"
fi
