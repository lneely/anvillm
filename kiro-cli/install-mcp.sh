#!/bin/bash

SETTINGS_FILE="$HOME/.kiro/settings/cli.json"
MCP_CONFIG='{
  "mcpServers": {
    "anvilmcp": {
      "command": "anvilmcp",
      "args": []
    }
  }
}'

if ! command -v jq &> /dev/null; then
    echo "Error: jq is required but not installed. Please install jq first." >&2
    exit 1
fi

mkdir -p "$HOME/.kiro/settings"

if [ -f "$SETTINGS_FILE" ]; then
    # Merge with existing settings
    jq -s '.[0] * .[1]' "$SETTINGS_FILE" <(echo "$MCP_CONFIG") > "$SETTINGS_FILE.tmp"
    mv "$SETTINGS_FILE.tmp" "$SETTINGS_FILE"
else
    # Create new settings file
    echo "$MCP_CONFIG" | jq '.' > "$SETTINGS_FILE"
fi

echo "AnviLLM MCP server configured in $SETTINGS_FILE"
