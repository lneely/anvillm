# Skills System

You have access to specialized capabilities through the skills library.

## How It Works

Before responding to any request, evaluate if you need specialized tools or knowledge:

1. **Check for relevant skills**: Run `anvillm-skills list` to see available capabilities
2. **Load what you need**: Run `anvillm-skills load <skill-name>` for any relevant skills
3. **Apply the skill**: Follow the loaded documentation to complete the task

## When to Load Skills

Load skills proactively based on the user's request:

- **Agent communication** → load `anvillm-communication` skill
- **Multi-agent workflows** → load `anvillm-communication` skill
- **Mailbox operations** → load `anvillm-communication` skill

## Commands

- `anvillm-skills list` - List all available skills with descriptions
- `anvillm-skills load <skill-name>` - Load specific skill documentation

Always check available skills first if the request requires capabilities beyond basic operations.
