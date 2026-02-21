# Skills System

You have access to specialized capabilities through the skills library.

## How It Works

Before responding to any request, evaluate if you need specialized tools or knowledge:

1. **Check for relevant skills**: Run `anvillm-skills list` to see available capabilities
2. **Load what you need**: Run `anvillm-skills load <skill-name>` for any relevant skills
3. **Apply the skill**: Follow the loaded documentation to complete the task

## When to Load Skills

Always check if a skill exists for the operation you're about to perform:

1. Run `anvillm-skills list` to see available skills
2. If a skill matches the domain (beads, communication, GitHub, etc.), load it first
3. Follow the skill's guidance for the operation

Common patterns:
- Bead/task management operations → `beads` skill
- Agent communication/mailbox → `anvillm-communication` skill
- GitHub operations → `github-cli` skill
- Web development with browser → `web-dev-browser-screencapture` skill

## Commands

- `anvillm-skills list` - List all available skills with descriptions
- `anvillm-skills load <skill-name>` - Load specific skill documentation

Always check available skills first if the request requires capabilities beyond basic operations.
