#!/bin/bash

SETTINGS_FILE="$HOME/.config/anvillm/claude/settings.json"
HOOKS_CONFIG='{
  "hooks": {
    "UserPromptSubmit": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "$HOME/.config/anvillm/claude/hooks/anvillm-state-running.sh"
          }
        ]
      }
    ],
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "$HOME/.config/anvillm/claude/hooks/anvillm-state-idle.sh"
          }
        ]
      }
    ]
  }
}'

if ! command -v jq &> /dev/null; then
    echo "Error: jq is required but not installed. Please install jq first." >&2
    exit 1
fi

mkdir -p "$HOME/.config/anvillm/claude"

if [ -f "$SETTINGS_FILE" ]; then
    # Merge with existing settings
    jq -s '.[0] * .[1]' "$SETTINGS_FILE" <(echo "$HOOKS_CONFIG") > "$SETTINGS_FILE.tmp"
    mv "$SETTINGS_FILE.tmp" "$SETTINGS_FILE"
else
    # Create new settings file
    echo "$HOOKS_CONFIG" | jq '.' > "$SETTINGS_FILE"
fi

echo "Claude hooks installed to $SETTINGS_FILE"

# Install agent configuration (includes Output Protocol + mailbox instructions)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
mkdir -p "$HOME/.config/anvillm/claude/agents"
cp "$SCRIPT_DIR/../agents/claude/anvillm-agent.md" "$HOME/.config/anvillm/claude/agents/anvillm-agent.md"
echo "Claude agent config installed to $HOME/.config/anvillm/claude/agents/anvillm-agent.md"
