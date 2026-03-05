#!/bin/bash
# capabilities: mcp
# description: Discover available MCP tools by capability and keyword
set -euo pipefail

if [ $# -lt 1 ]; then
  echo "Usage: discover_tool <keyword>" >&2
  exit 1
fi

keyword="$1"
results=""

# Search help index (path and description), resolve full path
if help_matches=$(9p read agent/tools/help | grep -i "$keyword"); then
  while IFS=$'\t' read -r tool desc; do
    for cap in $(9p ls agent/tools | grep -v '^help$'); do
      if 9p ls "agent/tools/$cap" | grep -qx "$tool"; then
        results="${results:+$results
}tools/$cap/$tool	$desc"
        break
      fi
    done
  done <<< "$help_matches"
fi

# Search tool content for keyword
while IFS= read -r cap; do
  [ "$cap" = "help" ] && continue
  while IFS= read -r tool; do
    path="tools/$cap/$tool"
    echo "$results" | grep -q "^$path" && continue
    if 9p read "agent/$path" | grep -qi "$keyword"; then
      desc=$(9p read agent/tools/help | grep "^${tool}	" | cut -f2)
      [ -n "$desc" ] && results="${results:+$results
}$path	$desc"
    fi
  done < <(9p ls "agent/tools/$cap")
done < <(9p ls agent/tools)

if [ -n "$results" ]; then
  echo "$results" | sort -u
else
  echo "No tools found matching: $keyword"
fi
