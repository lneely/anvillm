# Ollama Backend for anvillm

The Ollama backend enables local LLM usage via [mcphost](https://github.com/mark3labs/mcphost) and Ollama.

## Prerequisites

1. **Ollama**: Install and run Ollama
   ```bash
   # Install from https://ollama.ai
   ollama serve
   
   # Pull a model (default: qwen3:8b)
   ollama pull qwen3:8b
   ```

2. **mcphost**: Install from mark3labs
   ```bash
   go install github.com/mark3labs/mcphost@latest
   ```

## Configuration

1. **MCP Configuration**: Copy template to config directory
   ```bash
   cp cfg/mcphost.json ~/.config/anvillm/mcphost.json
   ```

2. **Environment Variables** (optional):
   ```bash
   export OLLAMA_MODEL="ollama:qwen3:8b"  # Override default model
   export MCPHOST_CONFIG="$HOME/.config/anvillm/mcphost.json"  # Override config path
   export OLLAMA_HOST="http://localhost:11434"  # Override Ollama URL
   ```

## Usage

Start anvilsrv and create an Ollama session:

```bash
# Via 9P interface
echo "new ollama" | 9p write agent/sessions/ctl

# Send prompts
echo "prompt <session-id> What is 2+2?" | 9p write agent/sessions/ctl
```

## Known Limitations

- **MCP Tools Only**: Cannot execute arbitrary system commands directly
- **Local Model Performance**: Slower than cloud-based models
- **Requires Local Setup**: Ollama must be installed and running
- **Model Loading Time**: Initial startup takes ~30 seconds for model loading
- **Limited Capabilities**: Functionality depends on configured MCP servers

## MCP Servers

The default configuration includes:

- **filesystem**: Read/write files in HOME directory
- **bash**: Execute bash commands
- **todo**: Manage ephemeral todos
- **http**: Fetch web content
- **anvilmcp**: 9P integration for beads

## Troubleshooting

**Ollama not running:**
```bash
ollama serve
```

**Model not found:**
```bash
ollama pull qwen3:8b
```

**Config not found:**
```bash
cp cfg/mcphost.json ~/.config/anvillm/mcphost.json
```

**Check backend status:**
```bash
9p read agent/sessions/list
```
