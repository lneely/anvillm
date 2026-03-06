#!/bin/bash

# Skip installation if claude command is not available
if ! command -v claude &> /dev/null; then
    echo "Skipping MCP server installation: 'claude' command not found in PATH"
    exit 0
fi

# Install anvilmcp MCP server
if ! claude mcp get anvilmcp &> /dev/null; then
    claude mcp add --scope user --transport stdio anvilmcp -- anvilmcp
    echo "anvilmcp installed"
fi

# Install superpowers-mcp-server if available
if command -v superpowers-mcp-server &> /dev/null; then
    if ! claude mcp get superpowers-mcp-server &> /dev/null; then
        claude mcp add --scope user --transport stdio superpowers-mcp-server -- superpowers-mcp-server
        echo "superpowers-mcp-server installed"
    fi
fi

echo "MCP servers configured"
