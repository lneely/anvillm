# AnviLLM

LLM orchestrator using 9P — scriptable, multi-backend, crash-resilient

## Architecture

![Architecture](docs/diagrams/architecture.svg?v=2)

**Core:** anvillm (9P daemon), anvilmcp (optional MCP server, see [anvillm-mcp](https://github.com/lneely/anvillm-mcp))
**Frontends:** Any 9P-speaking program — currently Assist (Acme)
**Benefits:** Shared sessions, crash recovery, scriptable via 9P, cross-backend agent communication

## Why 9P?

9P turns orchestration into file operations (`read`, `write`, `ls`, `stat`). Control agents with standard tools: `cat`, `echo`, shell scripts, or any language with file I/O. Compose workflows with Unix pipes, `grep`, `awk`, `jq`. Only requirement: a 9P client (plan9port's `9p` or compatible). Built with [9fans.net/go](https://9fans.net/go).

## Requirements

Go 1.21+, [plan9port](https://github.com/lneely/plan9port) (wayland branch, provides `9pfuse` with truncate fix), tmux, [landrun](https://github.com/zouuup/landrun) (kernel 5.13+), backend ([Claude Code](https://github.com/anthropics/claude-code), [Kiro](https://kiro.dev), or [Ollama](https://ollama.com) + [ollie](https://github.com/lneely/ollie))

## Installation

```sh
git clone https://github.com/lneely/anvillm && cd anvillm && mk
```

**Service integration** (optional, see `services/*/README.md`):
```sh
# systemd user
cp services/systemd/anvillm-user.service ~/.config/systemd/user/
systemctl --user enable --now anvillm

# systemd system
sudo cp services/systemd/anvillm.service /etc/systemd/system/
sudo systemctl enable --now anvillm

# runit
sudo cp -r services/runit /etc/sv/anvillm
sudo ln -s /etc/sv/anvillm /var/service/
```

## Usage

```sh
anvillm start       # background
anvillm fgstart     # foreground
anvillm status
anvillm stop
```

On startup, the server automatically mounts at `~/mnt/anvillm` via 9pfuse.

`Assist` auto-starts if needed.

**Namespaces:** Run multiple instances via `$NAMESPACE` (default: `/tmp/ns.$USER.:0`)
```sh
NAMESPACE=/tmp/ns.$USER.:1 anvillm start
NAMESPACE=/tmp/ns.$USER.:1 Assist
```

### Frontends

Any program that speaks 9P can be a frontend. The 9P filesystem exposes session management, state, messaging, and configuration as plain files — so building a new frontend is just reading and writing files.

- **Assist** — Acme client ([anvillm-acme](https://github.com/lneely/anvillm-acme))

For web frontends that can't speak 9P directly, `anvilwebgw` bridges HTTP to 9P.

Helper scripts served via 9P at `tools/` provide building blocks for frontends:

```sh
bash <(9p read tools/<scriptname>)
```

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `NAMESPACE` | `/tmp/ns.$USER.:0` | 9P namespace for server/client communication |
| `ANVILLM_BEADS_PATH` | `~/.beads` | Beads database location (used by 9beads) |
| `ANVILLM_TERMINAL` | `foot` | Terminal command for tmux attach |
| `ANTHROPIC_API_KEY` | — | Claude API key (optional if using `claude /login`) |
| `CLAUDE_AGENT_NAME` | `anvillm-agent` | Claude agent configuration name |
| `KIRO_API_KEY` | — | Kiro API key (optional if using `kiro-cli login`) |
| `ANVILLM_OLLAMA_MODEL` | `qwen3:8b` | Ollama model to use for ollama backend |
| `ANVILLM_SKILLS_DIR` | `$CLAUDE_CONFIG_DIR/skills:~/.kiro/skills:~/.config/anvillm/skills` | Colon-separated skill directories (searched in order) |

### Skills System

Skills are loaded from multiple directories via the `anvillm/skills` 9pfs. By default, searches:
1. `$CLAUDE_CONFIG_DIR/skills` (if `CLAUDE_CONFIG_DIR` set)
2. `~/.kiro/skills`
3. `~/.config/anvillm/skills`

Override with `ANVILLM_SKILLS_DIR` (colon-separated paths). Skills are organized by intent (virtual directories) and discovered via `SKILL.md` front-matter.

**Usage:**
```sh
9p ls anvillm/skills                    # list intents
9p read anvillm/skills/help             # search index
9p read anvillm/skills/tasks/beads/SKILL.md
```

### Sandbox Config Templates

Available in sandbox YAML files (`~/.config/anvillm/`):

| Template | Expands To | Description |
|----------|------------|-------------|
| `{CWD}` | Current working directory | Session's working directory |
| `{HOME}` | User home directory | `$HOME` |
| `{TMPDIR}` | Temp directory | `$TMPDIR` or `/tmp` |
| `{XDG_CONFIG_HOME}` | XDG config | `$XDG_CONFIG_HOME` or `~/.config` |
| `{XDG_DATA_HOME}` | XDG data | `$XDG_DATA_HOME` or `~/.local/share` |
| `{XDG_CACHE_HOME}` | XDG cache | `$XDG_CACHE_HOME` or `~/.cache` |
| `{XDG_STATE_HOME}` | XDG state | `$XDG_STATE_HOME` or `~/.local/state` |
| `{ANY_ENV_VAR}` | Environment variable | Any `$ENV_VAR` from the environment |

Templates use `{VARNAME}` syntax. Any environment variable can be referenced.

## Backends & Sandboxing

**Backends:** Claude (`npm install -g @anthropic-ai/claude-code`), Kiro ([kiro.dev](https://kiro.dev)), Ollama (local models via [ollie](https://github.com/lneely/ollie))

**Sandbox:** [landrun](https://github.com/zouuup/landrun) (always enabled) — Defaults: CWD/`/tmp`/config (rw), `/usr`/`/lib`/`/bin` (ro+exec), no network

**Config** (`~/.config/anvillm/`): Layered YAML files, most permissive wins:

<p align="center"><img src="docs/diagrams/sandbox-config.svg?v=2" width="250"></p>

- Default sandbox: `sandbox/default.yaml`

```yaml
network: {enabled: true, unrestricted: true}
filesystem: {rw: ["{CWD}", "{HOME}/.npm"]}
```

**Templates:** `{CWD}`, `{HOME}`, `{TMPDIR}`, `{XDG_*}` (see Configuration)

**Kernel requirements:** 5.13+ (Landlock v1), 6.7+ (v4), 6.10+ (v5 network)
Set `best_effort: true` for unsandboxed fallback (⚠️ if no Landlock support)

**Session lifecycle:**

<p align="center"><img src="docs/diagrams/session-lifecycle.svg?v=2" width="500"></p>

State transitions: `idle` ↔ `running` cycle via CLI hooks (`userPromptSubmit` when user sends prompt, `stop` when agent finishes). Crash → `error` → auto-restart → `starting`. Note: any state can transition to `stopped` or `killed` (not shown); `stopped` can restart → `starting`.

**Self-healing:** Auto-restarts crashes every 5s (preserves context/alias/cwd), skips intentional stops

<p align="center"><img src="docs/diagrams/crash-recovery.svg?v=2" width="500"></p>

Restored sessions automatically resume the latest conversation (kiro: `-r`, claude: `-c`).

**Daemon recovery:** If the daemon itself crashes but tmux sessions are still running, use `Recover` in Assist or manually restore sessions.

**Add backend:** Implement `CommandHandler`/`StateInspector` in `internal/backends/yourbackend.go`, register in `main.go`

### Ollama Backend

Run local LLMs via Ollama.

**Requirements:**
1. **Ollama:** Install from [ollama.com](https://ollama.com)
   ```sh
   curl -fsSL https://ollama.com/install.sh | sh
   ollama serve
   ollama pull qwen2.5-coder:7b
   ```

2. **Ollie CLI:** Install from [github.com/lneely/ollie](https://github.com/lneely/ollie)
   ```sh
   git clone https://github.com/lneely/ollie && cd ollie && go install
   ```

**Usage:**
```sh
echo 'new ollama /path/to/project' | 9p write anvillm/ctl
```

**Configuration:** Set model via `ANVILLM_OLLAMA_MODEL` (default: `qwen3:8b`)
```sh
ANVILLM_OLLAMA_MODEL=llama3.2 echo 'new ollama /path' | 9p write anvillm/ctl
```

## 9P Filesystem

`$NAMESPACE/agent`:

```
anvillm/
├── ctl             # "new <backend> <cwd>" creates session
├── list            # id, alias, state, pid, cwd
├── events          # Event stream (state changes, messages)
└── <id>/
    ├── ctl         # "stop", "restart", "kill"
    ├── state       # starting, idle, running, stopped, error, exited
    ├── context     # Prepended to prompts (r/w)
    ├── alias       # Session name (r/w)
    ├── pid         # Process ID
    ├── cwd         # Working directory
    ├── backend     # Backend name
    ├── role        # Role name
    ├── tasks       # Task names
    ├── tmux        # Tmux session name
    ├── inbox       # Incoming messages (JSON)
    ├── outbox      # Outgoing messages (JSON)
    ├── completed   # Archived messages (JSON, "Archive" in Assist)
    └── mail        # Write messages (convenience)
```

**Client Interactions:**

<p align="center"><img src="docs/diagrams/client-interactions.svg?v=2" width="400"></p>

Different clients interact with different parts of the filesystem: frontends read state, control files manage sessions, scripts consume events.

### Beads

Task tracking is provided by the separate [9beads](https://github.com/lneely/9beads) service, which exposes a `beads/` 9P filesystem.

### Events & Mailbox

**Mailbox Flow:**

![Mailbox Flow](docs/diagrams/mailbox-flow.svg?v=2)

Cross-backend communication: messages route between any participants (user, Claude agents, Kiro agents, Ollama agents) via the mailbox system.

**Examples:**

```sh
# Events
9p read anvillm/events  # {"type":"state_change","session_id":"...","state":"running",...}

# Mailbox
echo '{"to":"a3f2b9d1","type":"REVIEW_REQUEST","subject":"...","body":"..."}' | 9p write anvillm/b4e3c8f2/mail
9p read anvillm/a3f2b9d1/inbox
9p read anvillm/a3f2b9d1/completed
```

### Basic Session Example

```sh
# Create session
echo 'new claude /home/user/project' | 9p write anvillm/ctl

# List sessions (newest first)
9p read anvillm/list

# Get most recent session ID
ID=$(9p read anvillm/list | head -1 | awk '{print $1}')

# Send prompt via mailbox
echo '{"to":"'$ID'","type":"PROMPT_REQUEST","subject":"User prompt","body":"Hello"}' | 9p write user/mail

# Check state
9p read anvillm/$ID/state

# Read response from inbox
9p read user/inbox
```

See `SECURITY.md`

## Spawning Agents

`anvilspawn` creates a new agent session with a given backend, role, and working directory:

```sh
anvilspawn <backend> <role> [workdir]
```

For example:

```sh
anvilspawn kiro developer /path/to/project
anvilspawn claude reviewer
```

If `workdir` is omitted, it defaults to the current directory. The script creates the session, assigns an alias derived from the directory and role, and sets the role. The agent ID is printed to stdout.

## Roles

Roles define agent behavior and constraints. Each role is a Markdown file in `~/.config/anvillm/roles/` with YAML front-matter:

```yaml
---
name: Developer
description: Code implementation agent
focus-areas: coding, development, implementation
worker: true
---

You are a developer. Your ONLY job is to write code. ...
```

**Front-matter fields:**
- `name` — display name
- `description` — what the agent does
- `focus-areas` — comma-separated areas of responsibility
- `worker: true` — marks the role as eligible for autonomous nudging by `anvillm-supervisor`

The body of the file is injected as agent context, defining the agent's responsibilities and constraints. Assign a role to a session by writing to its `role` file:

```sh
echo "developer" | 9p write anvillm/$ID/role
```

Or use `anvilspawn`, which sets the role automatically.

**Available roles:** `developer`, `solo-developer`, `reviewer`, `tester`, `researcher`, `devops`, `pkgmgr`, `author`, `technical-editor`, `conductor`

## Autonomous Workflows

There are two approaches to autonomous workflows:

### 1. Supervisor (cron-based)

`anvillm-supervisor` runs as a cron job and performs periodic maintenance:

- `--nudge` — sends work prompts to idle sessions whose role has `worker: true` in its front-matter
- `--orphans` — unclaims beads assigned to sessions that no longer exist
- `--auto-mount <workdir>` — mounts a beads database for a working directory

Install the cron jobs:
```sh
mk cron-install
```

This is a lightweight, hands-off approach: spawn workers with `anvilspawn`, and the supervisor keeps them busy as long as there are ready beads.

### 2. Conductor (agent-based)

The Conductor role is an orchestration agent that decomposes goals into beads, spawns workers, and coordinates them to completion. Unlike the supervisor, the Conductor actively plans and adapts.

```sh
anvilspawn --role conductor kiro /path/to/project
CONDUCTOR_ID=$(9p read anvillm/list | head -1 | awk '{print $1}')

echo '{"to":"'$CONDUCTOR_ID'","type":"WORK_REQUEST","subject":"Execute","body":"Complete bead '$BEAD_ID'"}' | 9p write user/mail
```

The Conductor analyzes dependencies, spawns specialized bots, and delegates work in parallel. Agents notify the Conductor when blocked; it signals them to resume when dependencies resolve.

![Autonomous Workflow](docs/diagrams/automation-workflow.svg?v=3)

**Monitoring:**
- Event stream: `9p read anvillm/events` (state changes, messages)
- Debug logs: `~/.config/anvillm/logs/` (set `ANVILLM_DEBUG=1` for verbose output)
- Foreground mode: `anvillm fgstart` for live stderr output

## MCP Integration

`anvilmcp` ([anvillm-mcp](https://github.com/lneely/anvillm-mcp)) provides sandboxed code execution for MCP clients (Claude Desktop, Kiro, etc.).

**Note:** anvilmcp is optional. Tools are served by anvillm via 9P and can be invoked directly:

```sh
bash <(9p read anvillm/tools/check_inbox.sh)
```

anvilmcp adds sandbox isolation (landlock/landrun) around execution for MCP clients.

**Install:** See [anvillm-mcp](https://github.com/lneely/anvillm-mcp) for backend-specific setup.

See [Code Execution User Guide](docs/code-execution-user-guide.md) for details.

## Integrations

| Project | Description |
|---------|-------------|
| [9beads](https://github.com/lneely/9beads) | Task management via 9P — beads-based workflow tracking |
| [anvillm-acme](https://github.com/lneely/anvillm-acme) | Acme frontend for session management |
| [9beads-acme](https://github.com/lneely/9beads-acme) | Acme frontend for 9beads task management |
| [anvillm-mcp](https://github.com/lneely/anvillm-mcp) | MCP server with sandboxed execution |
| [agent-skills](https://github.com/lneely/agent-skills) | Discoverable skill definitions for agents |

## Troubleshooting

| Problem | Solution |
|---------|----------|
| Can't connect | `anvillm status`; try `anvillm start` |
| Session won't start | Check stderr; verify backend installed |
| Landlock ABI error | Set `best_effort: true` or upgrade kernel |
| Permission denied | Add paths to layered config |
| Orphaned tmux | `tmux kill-session -t anvillm-0` |
| 9P not working | `9p ls agent` |
| Stale PID | `anvillm stop` auto-cleans |
| Daemon won't stop | `anvillm fgstart` for logs |
| Bot stuck in "running" | Attach to the session's tmux window and type "stop work." to interrupt it and return to idle |
See Configuration section for environment variables.
