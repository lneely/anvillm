# AnviLLM

9P-based LLM orchestrator. Front-ends: Acme, Emacs, TUI, web.

## Architecture

**Core:** anvilsrv (9P daemon), anvilmcp (MCP server)  
**Clients:** Assist (Acme), anvillm.el (Emacs), anvillm (TUI), anvilweb (web)  
**Benefits:** Shared sessions, crash-resilient, scriptable via 9P, service integration, MCP support

## Requirements

Go 1.21+, plan9port, tmux, [landrun](https://github.com/zouuup/landrun) (kernel 5.13+), backend ([Claude Code](https://github.com/anthropics/claude-code) or [Kiro](https://kiro.dev))

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
sudo cp services/systemd/anvilsrv.service /etc/systemd/system/ && sudo systemctl enable --now anvilsrv

# runit
sudo cp -r services/runit /etc/sv/anvilsrv && sudo ln -s /etc/sv/anvilsrv /var/service/
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
**Namespaces:** Run multiple instances via `$NAMESPACE` (default: `/tmp/ns.$USER.:0`)
```sh
NAMESPACE=/tmp/ns.$USER.:1 anvilsrv start  # :1
NAMESPACE=/tmp/ns.$USER.:1 Assist          # connect
```

### Clients

**TUI:** `anvillm` — Keys: `s` start | `p` prompt | `t` stop | `R` restart | `K` kill | `a` alias | `r` refresh | `d` daemon | `?` help | `q` quit

**Emacs:** `(require 'anvillm)` then `M-x anvillm` — Keys: `s` start | `p` prompt | `P` prompt (minibuffer) | `t` stop | `R` restart | `K` kill | `a` alias | `r`/`g` refresh | `d` daemon | `q` quit | `?` help

**Web:** `anvilweb` (port :8080) — ⚠️ NO auth, localhost only. Remote: `ssh -L 8080:localhost:8080 user@remote`

**Acme:** Type `Assist` and middle-click

**Acme tag:** `Get Attach Stop Restart Kill Alias Context Log Daemon Inbox Archive`

| Command | Action |
|---------|--------|
| `Kiro <dir>` / `Claude <dir>` | Start session |
| `Stop <id>` / `Restart <id>` / `Kill <id>` | Control session |
| `Attach <id>` | Open tmux |
| `Alias <id> <name>` | Name session |
| `Context <id>` | Edit context |
| `Log <id>` | View log |
| `Daemon` | Manage daemon |
| `Inbox [id]` / `Archive [id]` | View messages |

Right-click ID for prompt window. 2-1 chord for fire-and-forget.

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
    ├── in          # Write prompts
    ├── out         # Read responses
    ├── ctl         # "stop", "restart", "kill"
    ├── state       # starting, idle, running, stopped, error, exited
    ├── context     # Prepended to prompts (r/w)
    ├── alias       # Session name (r/w)
    ├── pid         # Process ID
    ├── cwd         # Working directory
    ├── backend     # Backend name
    ├── inbox       # Incoming messages (JSON)
    └── archive     # Archived messages (JSON)
```

### Beads

Persistent task tracking via [beads](https://github.com/steveyegge/beads). Dependency-aware graph.

```sh
echo 'init' | 9p write agent/beads/ctl
echo 'new "Implement login" "Add JWT auth"' | 9p write agent/beads/ctl
echo 'claim bd-a1b2' | 9p write agent/beads/ctl
echo 'complete bd-a1b2' | 9p write agent/beads/ctl
echo 'dep bd-child bd-parent' | 9p write agent/beads/ctl  # child blocks parent
9p read agent/beads/ready | jq -r '.[] | "\(.id): \(.title)"'
```

Config: `~/.beads/` (override: `ANVILLM_BEADS_PATH`). Shared across namespaces. See `internal/beads/README.md`.

### Events & Mailbox

```sh
# Events
9p read agent/events  # {"type":"state_change","session_id":"...","state":"running",...}

# Mailbox
echo '{"to":"a3f2b9d1","type":"REVIEW_REQUEST","subject":"...","body":"..."}' | 9p write agent/b4e3c8f2/mail
9p read agent/a3f2b9d1/inbox
9p read agent/a3f2b9d1/archive
```

### Example

```sh
echo 'new claude /home/user/project' | 9p write agent/ctl
echo 'Hello' | 9p write agent/a3f2b9d1/in
9p read agent/a3f2b9d1/state  # "running"
```

See `SECURITY.md`.

## Backends & Sandboxing

**Backends:** Claude (`npm install -g @anthropic-ai/claude-code`), Kiro ([kiro.dev](https://kiro.dev)). Both run with full permissions.

**Add:** Implement `CommandHandler`/`StateInspector` in `internal/backends/yourbackend.go`, register in `main.go`.

**Sandbox:** [landrun](https://github.com/zouuup/landrun) (cannot disable). Defaults: CWD/`/tmp`/config (rw), `/usr`/`/lib`/`/bin` (ro+exec), no network.

**Config** (`~/.config/anvillm/`): `global.yaml` → `backends/<name>.yaml` → `roles/<name>.yaml` (default: `roles/default.yaml`) → `tasks/<name>.yaml`. Most permissive wins.

```yaml
network: {enabled: true, unrestricted: true}
filesystem: {rw: ["{CWD}", "{HOME}/.npm"]}
```

Templates: `{CWD}`, `{HOME}`, `{TMPDIR}`. Tip: Make `roles/default.yaml` permissive.

**Kernel:** 5.13+ (Landlock v1), 6.7+ (v4), 6.10+ (v5 network). Best-effort: `best_effort: true` (⚠️ unsandboxed if no Landlock).

**Self-healing:** Auto-restarts crashes every 5s (preserves context/alias/cwd). Skips intentional stops.

## Workflows

**Skills:** `anvillm-skills list`, `anvillm-skills load anvillm-communication`

**Examples:** `./workflows/DevReview claude /path` (dev/reviewer loop), `./workflows/Planning kiro /path` (research/eng/editor)

**Pattern:**

```sh
echo "new claude /project" | 9p write agent/ctl
ID=$(9p read agent/list | tail -1 | awk '{print $1}')
echo "You are..." | 9p write agent/$ID/context
echo '{"to":"...","type":"...","subject":"...","body":"..."}' | 9p write agent/$ID/mail
echo 'new "Task"' | 9p write agent/beads/ctl
```

See `workflows/`, `kiro-cli/SKILLS_PROMPT.md`.

## MCP Integration

`anvilmcp` exposes AnviLLM via Model Context Protocol for Claude Desktop, Cline, etc.

**Install:**
```sh
./kiro-cli/install-mcp.sh  # Adds to ~/.kiro/settings/cli.json
```

**Tools:** `read_inbox`, `send_message`, `list_sessions`, `set_state`

Enables LLM clients to control AnviLLM sessions and communicate with agents.

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

Debug: `anvilsrv fgstart`  
Terminal: `$ANVILLM_TERMINAL` (default: `foot`)
