#!/bin/bash

# Target .mcp.json file in project root
MCP_FILE=".mcp.json"

# MCP configuration for anvilmcp
MCP_CONFIG='{
  "mcpServers": {
    "anvilmcp": {
      "type": "stdio",
      "command": "anvilmcp",
      "args": [],
      "env": {}
    }
  }
}'

if ! command -v jq &> /dev/null; then
    echo "Error: jq is required but not installed. Please install jq first." >&2
    exit 1
fi

if [ -f "$MCP_FILE" ]; then
    # Merge with existing .mcp.json
    jq -s '.[0] * .[1]' "$MCP_FILE" <(echo "$MCP_CONFIG") > "$MCP_FILE.tmp"
    mv "$MCP_FILE.tmp" ~/"$MCP_FILE"
else
    # Create new .mcp.json file
    echo "$MCP_CONFIG" | jq '.' > ~/"$MCP_FILE"
fi

echo "AnviLLM MCP server configured in $MCP_FILE"
echo ""
echo "The server provides these tools:"
echo "  - read_inbox    : Read messages from agent's inbox"
echo "  - send_message  : Send messages to other agents or user"
echo "  - list_sessions : List all active sessions"
echo "  - set_state     : Set agent state (idle, running, etc.)"
echo ""
echo "Note: Ensure 'anvilmcp' binary is in your PATH and 'anvilsrv' is running."
