#!/bin/bash

SETTINGS_FILE="$HOME/.claude/settings.json"
HOOKS_CONFIG='{
  "hooks": {
    "UserPromptSubmit": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "$HOME/.claude/hooks/anvillm-state-running.sh"
          }
        ]
      }
    ],
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "$HOME/.claude/hooks/anvillm-state-idle.sh"
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

mkdir -p "$HOME/.claude"

if [ -f "$SETTINGS_FILE" ]; then
    # Merge with existing settings
    jq -s '.[0] * .[1]' "$SETTINGS_FILE" <(echo "$HOOKS_CONFIG") > "$SETTINGS_FILE.tmp"
    mv "$SETTINGS_FILE.tmp" "$SETTINGS_FILE"
else
    # Create new settings file
    echo "$HOOKS_CONFIG" | jq '.' > "$SETTINGS_FILE"
fi

echo "Claude hooks installed to $SETTINGS_FILE"
