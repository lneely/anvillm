#!/bin/bash

# Skip installation if claude command is not available
if ! command -v claude &> /dev/null; then
    echo "Skipping MCP server installation: 'claude' command not found in PATH"
    exit 0
fi

# Check if anvilmcp is already installed
if claude mcp get anvilmcp &> /dev/null; then
    echo "AnviLLM MCP server already installed"
    exit 0
fi

# Install anvilmcp MCP server using claude CLI (user scope)
claude mcp add --scope user --transport stdio anvilmcp -- anvilmcp

echo "AnviLLM MCP server configured in $MCP_FILE"
echo ""
echo "The server provides these tools:"
echo "  - read_inbox    : Read messages from agent's inbox"
echo "  - send_message  : Send messages to other agents or user"
echo "  - list_sessions : List all active sessions"
echo "  - set_state     : Set agent state (idle, running, etc.)"
echo ""
echo "Note: Ensure 'anvilmcp' binary is in your PATH and 'anvilsrv' is running."
