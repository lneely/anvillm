---
name: anvillm-sessions
intent: agents, sessions
description: Manage agent sessions (create, list, control). Use when spawning new agents, checking agent status, or controlling agent lifecycle.
---

# Anvillm Session Management

Create and control agent sessions.

## Commands

All commands use `execute_code` with `sandbox: "anvilmcp"`, EXCEPT spawn_agent.sh which requires `sandbox: "default"`.

List sessions:
```
Tool: execute_code
sandbox: anvilmcp
code: bash <(9p read agent/tools/list_sessions.sh)
```

Create session:
```
Tool: execute_code
sandbox: anvilmcp
code: bash <(9p read agent/tools/create_session.sh) <backend> <cwd> [sandbox=<sandbox>] [model=<model>]
```

Control session:
```
Tool: execute_code
sandbox: anvilmcp
code: bash <(9p read agent/tools/control_session.sh) <session-id> <command>
```

Spawn agent:
```
Tool: execute_code
sandbox: default
code: bash <(9p read agent/tools/spawn_agent.sh) <agent-id> [cwd-path] [initial-context-prompt]
```

Kill agent:
```
Tool: execute_code
sandbox: anvilmcp
code: bash <(9p read agent/tools/kill_agent.sh) [cwd-path]
```

## Backends

- `kiro-cli` - Kiro CLI agent
- `claude` - Claude CLI agent
- `ollama` - Ollama agent

## Session Lifecycle

1. Create: `create_session.sh kiro-cli /path/to/project sandbox=default`
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
