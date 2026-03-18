---
name: anvillm-sessions
intent: agents, sessions
description: Manage agent sessions (create, list, control). Use when spawning new agents, checking agent status, or controlling agent lifecycle.
---

# Anvillm Session Management

Create and control agent sessions.

## Commands

List sessions:
```
Tool: execute_code
tool: list_sessions.sh
```

Create session:
```
Tool: execute_code
tool: create_session.sh
args: ["--backend", "<backend>", "--cwd", "<cwd>", "--sandbox", "<sandbox>", "--model", "<model>"]
```

Control session:
```
Tool: execute_code
tool: control_session.sh
args: ["--session-id", "<session-id>", "--command", "<stop|restart|kill|refresh>"]
```

Spawn agent (requires default sandbox):
```
Tool: execute_code
tool: spawn_agent.sh
args: ["--agent-id", "<agent-id>", "--cwd", "<cwd-path>", "--prompt", "<initial-context>"]
sandbox: default
```
`--cwd` defaults to `$PWD`. `--prompt` is optional.

Kill agent:
```
Tool: execute_code
tool: kill_agent.sh
args: ["--agent-id", "<agent-id>"]
```

## Backends

- `kiro-cli` - Kiro CLI agent
- `claude` - Claude CLI agent
- `ollama` - Ollama agent

## Session Lifecycle

1. Create: `create_session.sh --backend kiro-cli --cwd /path/to/project --sandbox default`
2. Monitor: `list_sessions.sh` (check state field)
3. Control: `control_session.sh --session-id <id> --command stop|restart|kill|refresh`

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

## Session Handoff Pattern

Agent A dies. Orphan cron detects dead assignee, calls `unclaim`. `BeadReady` fires on the event bus with full bead JSON including comments. Any waiting bot receives it via `wait_for_bead.sh`, claims the bead, and resumes — no bootstrap step, no agent-written snapshot. State lives in beads + git.

## When NOT to Use

- Sending messages to agents (use built-in communication tools: `send_message.sh`, `check_inbox.sh`)
- Managing tasks (use beads skill)
