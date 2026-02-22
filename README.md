# AnviLLM

LLM orchestrator using 9P — scriptable, multi-backend, crash-resilient

## Architecture

![Architecture](docs/diagrams/architecture.svg?v=2)

**Core:** anvilsrv (9P daemon), anvilmcp (MCP server), anvilwebgw (API proxy)  
**Clients:** Assist (Acme), anvillm.el (Emacs), anvillm (TUI), anvilweb (web)  
**Benefits:** Shared sessions, crash recovery, scriptable via 9P, cross-backend agent communication

## Why 9P?

9P turns orchestration into file operations (`read`, `write`, `ls`, `stat`). Control agents with standard tools: `cat`, `echo`, shell scripts, or any language with file I/O. Compose workflows with Unix pipes, `grep`, `awk`, `jq`. Only requirement: a 9P client (plan9port's `9p` or compatible). Built with [9fans.net/go](https://9fans.net/go).

## Requirements

Go 1.21+, plan9port, tmux, [landrun](https://github.com/zouuup/landrun) (kernel 5.13+), backend ([Claude Code](https://github.com/anthropics/claude-code), [Kiro](https://kiro.dev), or [Ollama](https://ollama.com) + [ollie](https://github.com/lneely/ollie))

## Installation

```sh
git clone https://github.com/lneely/anvillm && cd anvillm && mk
```

**Service integration** (optional, see `services/*/README.md`):
```sh
# systemd user
cp services/systemd/anvilsrv-user.service ~/.config/systemd/user/
systemctl --user enable --now anvilsrv

# systemd system
sudo cp services/systemd/anvilsrv.service /etc/systemd/system/
sudo systemctl enable --now anvilsrv

# runit
sudo cp -r services/runit /etc/sv/anvilsrv
sudo ln -s /etc/sv/anvilsrv /var/service/
```

## Usage

```sh
anvilsrv start       # background
anvilsrv fgstart     # foreground
anvilsrv status
anvilsrv stop
```

`Assist` auto-starts if needed.

**Namespaces:** Run multiple instances via `$NAMESPACE` (default: `/tmp/ns.$USER.:0`)
```sh
NAMESPACE=/tmp/ns.$USER.:1 anvilsrv start
NAMESPACE=/tmp/ns.$USER.:1 Assist
```

### Clients

**TUI:** `anvillm` — Keys: `s` start | `p` prompt | `t` stop | `R` restart | `K` kill | `a` alias | `r` refresh | `d` daemon | `?` help | `q` quit

**Emacs:** `(require 'anvillm)` then `M-x anvillm` — Keys: `s` start | `p` prompt | `P` prompt (minibuffer) | `t` stop | `R` restart | `K` kill | `a` alias | `r`/`g` refresh | `d` daemon | `?` help | `q` quit

**Web:** `anvilweb` (port :8080) — ⚠️ NO auth, localhost only — Remote: `ssh -L 8080:localhost:8080 user@remote`

**Acme:** Type `Assist` and middle-click

**Tag commands:** `Get Attach Stop Restart Kill Alias Context Daemon Inbox Archive`

**Interaction:** Right-click ID for prompt window, 2-1 chord for fire-and-forget

| Command | Action |
|---------|--------|
| `Get` | Refresh session list |
| `Kiro <dir>` / `Claude <dir>` / `Ollama <dir>` | Start session |
| `Attach <id>` | Open tmux |
| `Stop <id>` / `Restart <id>` / `Kill <id>` | Control session |
| `Alias <id> <name>` | Name session |
| `Context <id>` | Edit context |
| `Daemon` | Manage daemon |
| `Inbox [id]` / `Archive [id]` | View messages |

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `NAMESPACE` | `/tmp/ns.$USER.:0` | 9P namespace for server/client communication |
| `ANVILLM_BEADS_PATH` | `~/.beads` | Beads database location (shared across namespaces) |
| `ANVILLM_TERMINAL` | `foot` | Terminal command for tmux attach (Assist) |
| `ANTHROPIC_API_KEY` | — | Claude API key (optional if using `claude /login`) |
| `CLAUDE_AGENT_NAME` | `anvillm-agent` | Claude agent configuration name |
| `KIRO_API_KEY` | — | Kiro API key (optional if using `kiro-cli login`) |
| `ANVILLM_OLLAMA_MODEL` | `qwen3:8b` | Ollama model to use for ollama backend |

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

- Default role: `roles/default.yaml`

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
echo 'new ollama /path/to/project' | 9p write agent/ctl
```

**Configuration:** Set model via `ANVILLM_OLLAMA_MODEL` (default: `qwen3:8b`)
```sh
ANVILLM_OLLAMA_MODEL=llama3.2 echo 'new ollama /path' | 9p write agent/ctl
```

## 9P Filesystem

`$NAMESPACE/agent`:

```
agent/
├── ctl             # "new <backend> <cwd>" creates session
├── list            # id, alias, state, pid, cwd
├── events          # Event stream (state changes, messages)
├── beads/          # Task tracking (persistent across namespaces)
│   ├── ctl         # Commands: init, new, claim, complete, fail, dep
│   ├── list        # All beads as JSON
│   ├── ready       # Ready beads (no blockers) as JSON
│   └── <bead-id>/
│       ├── status
│       ├── title
│       ├── description
│       ├── assignee
│       └── json
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

Persistent task tracking via [beads](https://github.com/steveyegge/beads) — Dependency-aware graph

```sh
echo 'init' | 9p write agent/beads/ctl
echo 'new "Implement login" "Add JWT auth"' | 9p write agent/beads/ctl
echo 'claim bd-a1b2' | 9p write agent/beads/ctl
echo 'complete bd-a1b2' | 9p write agent/beads/ctl
echo 'dep bd-child bd-parent' | 9p write agent/beads/ctl  # child blocks parent
9p read agent/beads/ready | jq -r '.[] | "\(.id): \(.title)"'
```

Config: `~/.beads/` (override: `ANVILLM_BEADS_PATH`) — Shared across namespaces — See `internal/beads/README.md`

### Events & Mailbox

**Mailbox Flow:**

![Mailbox Flow](docs/diagrams/mailbox-flow.svg?v=2)

Cross-backend communication: messages route between any participants (user, Claude agents, Kiro agents, Ollama agents) via the mailbox system.

**Examples:**

```sh
# Events
9p read agent/events  # {"type":"state_change","session_id":"...","state":"running",...}

# Mailbox
echo '{"to":"a3f2b9d1","type":"REVIEW_REQUEST","subject":"...","body":"..."}' | 9p write agent/b4e3c8f2/mail
9p read agent/a3f2b9d1/inbox
9p read agent/a3f2b9d1/completed
```

### Basic Session Example

```sh
# Create session
echo 'new claude /home/user/project' | 9p write agent/ctl

# List sessions (newest first)
9p read agent/list

# Get most recent session ID
ID=$(9p read agent/list | head -1 | awk '{print $1}')

# Send prompt via mailbox
echo '{"to":"'$ID'","type":"PROMPT_REQUEST","subject":"User prompt","body":"Hello"}' | 9p write user/mail

# Check state
9p read agent/$ID/state

# Read response from inbox
9p read user/inbox
```

See `SECURITY.md`

## Bot Templates

Bot templates provide pre-configured agents for common tasks. Templates are shell scripts that create sessions with specialized context and configuration.

**Available Templates:**
- `Taskmaster` — Converts project plans into dependency-aware task graphs (beads)
- `Conductor` — Orchestrates parallel execution by spawning and coordinating specialized agents

**Usage:**
```sh
./bot-templates/Taskmaster kiro /path/to/project
./bot-templates/Conductor kiro /path/to/project
```

Templates set up context, roles, and initial configuration. Interact via any client (Assist, TUI, Emacs, Web) or directly via 9P mailbox.

**Creating Templates:** Shell scripts that write to `agent/ctl` and configure `context`, `role`, `tasks`. See existing templates for examples.

## Autonomous Workflows

Automate workflows with Taskmaster and Conductor bots. Input project plan → Taskmaster creates tasks with dependencies → Conductor orchestrates parallel execution.

<p align="center"><img src="docs/diagrams/automation-workflow.svg?v=3" width="600"></p>

Conductor receives the top-level bead ID, analyzes dependencies, and spawns agents to work in parallel. Agents notify Conductor when blocked; Conductor signals them to resume when dependencies resolve.

### Usage

Examples use raw 9P calls. These actions can also be performed from any front-end (Assist, TUI, Emacs, Web).

**1. Taskmaster: Create tasks from project plan**

```sh
./bot-templates/Taskmaster kiro /path/to/project
TASKMASTER_ID=$(9p read agent/list | head -1 | awk '{print $1}')

# Send project plan
echo '{"to":"'$TASKMASTER_ID'","type":"PROMPT_REQUEST","subject":"Create tasks","body":"<your project plan>"}' | 9p write user/mail

# Get top-level bead ID
BEAD_ID=$(9p read agent/beads/list | jq -r '.[0].id')
```

**2. Conductor: Execute work**

```sh
./bot-templates/Conductor kiro /path/to/project
CONDUCTOR_ID=$(9p read agent/list | head -1 | awk '{print $1}')

# Send work request
echo '{"to":"'$CONDUCTOR_ID'","type":"WORK_REQUEST","subject":"Execute","body":"Complete bead '$BEAD_ID'"}' | 9p write user/mail
```

Conductor analyzes dependencies, spawns specialized bots, and delegates work in parallel.

**Monitoring:** Use any front-end (Assist, TUI, Emacs, Web). For custom integrations, see `EVENTS.md`.

## MCP Integration

`anvilmcp` exposes AnviLLM via Model Context Protocol for Claude Desktop, Cline, etc.

**Install:**
```sh
./kiro-cli/install-mcp.sh  # Adds to ~/.kiro/settings/cli.json
```

**Tools:** `read_inbox`, `send_message`, `list_sessions`, `set_state`

Enables LLM clients to discover agents and communicate via the mailbox system.

## Troubleshooting

| Problem | Solution |
|---------|----------|
| Can't connect | `anvilsrv status`; try `anvilsrv start` |
| Session won't start | Check stderr; verify backend installed |
| Landlock ABI error | Set `best_effort: true` or upgrade kernel |
| Permission denied | Add paths to layered config |
| Orphaned tmux | `tmux kill-session -t anvillm-0` |
| 9P not working | `9p ls agent` |
| Stale PID | `anvilsrv stop` auto-cleans |
| Daemon won't stop | `anvilsrv fgstart` for logs |
| anvilweb issues | Check anvilsrv running, namespace matches |

See Configuration section for environment variables.
