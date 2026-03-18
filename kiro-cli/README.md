# Kiro CLI Integration

This directory contains configuration for integrating AnviLLM with Kiro CLI.

## Installation

### MCP Server

Install the AnviLLM MCP server to enable agent communication tools:

```bash
./kiro-cli/install-mcp.sh
```

This configures the `anvilmcp` MCP server in `~/.kiro/settings/cli.json`, providing tools:
- `check_inbox` - Read messages from agent's inbox
- `send_message` - Send messages to other agents or user
- `list_sessions` - List all active sessions
- `set_state` - Set agent state (idle, running, etc.)

### Agent Configuration

The agent configuration is in `./kiro-cli/anvillm/anvillm-agent.json` and includes hooks that:
- Set agent state to `running` when prompts are submitted
- Set agent state to `idle` when stopped

To use this agent configuration, copy it to your Kiro agents directory:

```bash
cp ./kiro-cli/anvillm/anvillm-agent.json ~/.kiro/agents/
```

### Prompt Files

Copy the session-start prompt files so hooks can inject them:

```bash
mkdir -p ~/.kiro
cp ./kiro-cli/SKILLS_PROMPT.md ~/.kiro/SKILLS_PROMPT.md
cp ./OUTPUT_PROTOCOL.md ~/.kiro/OUTPUT_PROTOCOL.md
```

The `agentSpawn` hook injects both files at session start to set output discipline and skill discovery context.

## Requirements

- `anvilmcp` binary must be in PATH (built from `cmd/anvilmcp`)
- `anvilsrv` daemon must be running
- `jq` for installation scripts
