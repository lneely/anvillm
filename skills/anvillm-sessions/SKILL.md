---
name: anvillm-sessions
intent: agents, sessions
description: Manage agent sessions (create, list, control). Use when spawning new agents, checking agent status, or controlling agent lifecycle.
---

# Anvillm Session Management

Create and control agent sessions.

## Commands

- `list_sessions.sh` - List all active sessions (JSON)
- `create_session.sh <backend> <cwd> [role=<role>] [tasks=<task1,task2>] [model=<model>]` - Create new session
- `control_session.sh <session-id> <command>` - Control session (stop|restart|kill|refresh)

## Backends

- `kiro-cli` - Kiro CLI agent
- `claude` - Claude CLI agent
- `ollama` - Ollama agent

## Session Lifecycle

1. Create: `create_session.sh kiro-cli /path/to/project role=developer`
2. Monitor: `list_sessions.sh` (check state field)
3. Control: `control_session.sh <id> stop|restart|kill|refresh`

## Session States

- `starting` - Initializing
- `idle` - Ready for work
- `running` - Processing task
- `stopped` - Paused
- `error` - Failed
- `exited` - Terminated

## When to Use

- User asks to spawn/create a new agent
- Need to check agent status
- Need to stop/restart/kill an agent
- Coordinating multi-agent workflows

## When NOT to Use

- Sending messages to agents (use anvillm-communication skill)
- Managing tasks (use beads skill)
