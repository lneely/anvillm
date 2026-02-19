# AnviLLM

9P-based LLM orchestrator with multiple front-ends: Acme, Emacs, TUI, and web.

## Architecture

**Core:**
- **anvilsrv** - Background daemon exposing sessions via 9P filesystem
- **anvilmcp** - MCP server for Model Context Protocol integration

**Clients:**
- **Assist** - Acme UI (auto-starts daemon)
- **anvillm.el** - Emacs interface
- **anvillm** - Terminal UI (curses)
- **anvilweb** - Web interface

**Benefits:**
- Multiple clients share sessions
- Sessions survive client crashes
- Scriptable workflows via 9P
- Service management (systemd/runit)
- MCP integration (Claude Desktop, Cline, etc.)

## Requirements

- Go 1.21+, plan9port, tmux
- [landrun](https://github.com/zouuup/landrun) for sandboxing (Linux kernel 5.13+)
- Backend: [Claude Code](https://github.com/anthropics/claude-code) or [Kiro CLI](https://kiro.dev)

## Installation

```sh
git clone https://github.com/lneely/anvillm
cd anvillm
mk  # installs anvilsrv and Assist to $HOME/bin
```

### Service Integration

Install as system service (optional):

**systemd**:
```sh
# User service (recommended)
cp services/systemd/anvilsrv-user.service ~/.config/systemd/user/
systemctl --user enable --now anvilsrv

# System service
sudo cp services/systemd/anvilsrv.service /etc/systemd/system/
sudo systemctl enable --now anvilsrv
```

**runit**:
```sh
sudo cp -r services/runit /etc/sv/anvilsrv
sudo ln -s /etc/sv/anvilsrv /var/service/
```

See `services/*/README.md` for details.

## Usage

### Starting the Server

**Automatic**: `Assist` auto-starts `anvilsrv` if not running.

**Manual**:
```sh
anvilsrv start       # Daemonize (background)
anvilsrv fgstart     # Foreground (for debugging)
anvilsrv status      # Check if running
anvilsrv stop        # Shutdown
```

### Multiple Namespaces

Run multiple independent instances using `$NAMESPACE` (similar to `acme -a`).

**Default:** `/tmp/ns.$USER.:0`

**Example:**
```sh
# Start default instance (:0)
anvilsrv start

# Start second instance (:1)
NAMESPACE=/tmp/ns.$USER.:1 anvilsrv start

# Check status
anvilsrv status                           # :0
NAMESPACE=/tmp/ns.$USER.:1 anvilsrv status  # :1

# Connect Assist to specific instance
NAMESPACE=/tmp/ns.$USER.:1 Assist
```

Each instance has isolated PID file, 9P socket, and session list.

### Terminal UI (anvillm)

Curses-based interface (no Acme required):

```sh
anvillm
```

**Keys:** `s` start | `p` prompt | `t` stop | `R` restart | `K` kill | `a` alias | `r` refresh | `d` daemon | `?` help | `q` quit

**Navigation:** Arrow keys, vim (`j`/`k`), or emacs (`C-n`/`C-p`)

Attach to tmux session from another terminal with `tmux attach`.

### Emacs Interface (anvillm.el)

**Installation:**
```elisp
(add-to-list 'load-path "/path/to/anvillm.el/")
(require 'anvillm)
```

**Usage:** `M-x anvillm`

**Keys:** `s` start | `p` prompt | `P` prompt (minibuffer) | `t` stop | `R` restart | `K` kill | `a` alias | `r`/`g` refresh | `d` daemon | `q` quit | `?` help

**Navigation:** Standard Emacs (`n`, `p`, `C-n`, `C-p`). Click column headers to sort.

### Web Interface (anvilweb)

```sh
anvilweb              # starts on :8080
anvilweb -addr :3000  # custom port
```

**Features:** Start sessions, send prompts, edit context/aliases, stop/restart/kill. Auto-refreshes every 5 seconds.

**⚠️ Security Warning:**

**NO authentication.** Anyone with network access can control all sessions.

**Only run on localhost or trusted networks.** For remote access, use SSH port forwarding:

```sh
# Remote machine
anvilweb -addr localhost:8080

# Local machine
ssh -L 8080:localhost:8080 user@remote
# Browse to http://localhost:8080
```

Or use a reverse proxy with authentication (nginx, caddy).

### Acme Interface (Assist)

Type `Assist` in Acme and middle-click to open `/AnviLLM/` session manager.

**Main window tag:** `Get Attach Stop Restart Kill Alias Context Log Daemon Inbox Archive`

| Command | Description |
|---------|-------------|
| `Kiro <dir>` | Start kiro-cli session |
| `Claude <dir>` | Start Claude session |
| `Stop <id>` | Stop session (preserves tmux) |
| `Restart <id>` | Restart stopped session |
| `Kill <id>` | Terminate session |
| `Attach <id>` | Open tmux terminal |
| `Alias <id> <name>` | Name a session |
| `Context <id>` | Edit context (prepended to prompts) |
| `Log <id>` | View session log |
| `Daemon` | Manage anvilsrv daemon (start/stop/status) |
| `Inbox [id]` | View inbox messages (default: user) |
| `Archive [id]` | View archived messages (default: user) |

Right-click session ID to open prompt window. Select text and 2-1 chord on session ID for fire-and-forget prompts.

**Prompt window tag:** `Send`

## 9P Filesystem

`anvilsrv` exposes sessions at `$NAMESPACE/agent` (typically `/tmp/ns.$USER/agent`):

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

**Path Validation:** Session creation validates paths (e.g., `/../../../etc` → `/etc`) and rejects nonexistent directories.

### Task Tracking with Beads

Persistent, crash-resilient task tracking via [beads](https://github.com/steveyegge/beads). Provides dependency-aware task graph for multi-agent workflows.

**Initialize beads database:**
```sh
echo 'init' | 9p write agent/beads/ctl  # Uses prefix "bd" by default
```

**Create tasks:**
```sh
echo 'new "Implement login" "Add JWT authentication"' | 9p write agent/beads/ctl
```

**List ready tasks (no blockers):**
```sh
9p read agent/beads/ready | jq -r '.[] | "\(.id): \(.title)"'
```

**Claim and complete:**
```sh
echo 'claim bd-a1b2' | 9p write agent/beads/ctl
echo 'complete bd-a1b2' | 9p write agent/beads/ctl
```

**Add dependencies:**
```sh
echo 'dep bd-child bd-parent' | 9p write agent/beads/ctl  # child blocks parent
```

**Configuration:**
- Default: `~/.beads/`
- Override: `ANVILLM_BEADS_PATH=/custom/path`
- Shared across all namespaces

See `internal/beads/README.md` for details.

### Event Stream

Real-time session events at `/agent/events`:

```sh
9p read agent/events
# {"type":"state_change","session_id":"a3f2b9d1","state":"running","timestamp":"2026-02-19T22:00:00Z"}
# {"type":"message_received","session_id":"a3f2b9d1","from":"b4e3c8f2","timestamp":"2026-02-19T22:01:00Z"}
```

### Mailbox System

Inter-session messaging:

**Send message:**
```sh
cat > /tmp/msg.json <<'EOF'
{"to":"a3f2b9d1","type":"REVIEW_REQUEST","subject":"Review PR","body":"Please review..."}
EOF
9p write agent/b4e3c8f2/mail < /tmp/msg.json
```

**Read inbox:**
```sh
9p read agent/a3f2b9d1/inbox
```

**Archive:**
```sh
9p read agent/a3f2b9d1/archive
```

### Example Usage

```sh
echo 'new claude /home/user/project' | 9p write agent/ctl
echo 'Hello' | 9p write agent/a3f2b9d1/in
9p read agent/a3f2b9d1/state  # → "running"
echo 'stop' | 9p write agent/a3f2b9d1/ctl
9p read agent/a3f2b9d1/state  # → "stopped"
```

See `SECURITY.md` for 9P socket authentication details.

## Backends

| Backend | Install |
|---------|---------|
| Claude | `npm install -g @anthropic-ai/claude-code` |
| Kiro | [Kiro CLI](https://kiro.dev) |

Both run with full permissions (`--dangerously-skip-permissions`, `--trust-all-tools`).

**Adding backends:** Implement `CommandHandler` and `StateInspector` interfaces in `internal/backends/yourbackend.go`, register in `main.go`. See existing backends for examples.

## Self-Healing

`anvilsrv` monitors sessions every 5 seconds and auto-restarts crashed sessions (preserves context, alias, working directory). Rate-limited to prevent restart loops (minimum 5 seconds between attempts). Does not restart sessions stopped via `Stop` command.

Check logs with `anvilsrv fgstart` to see auto-restart activity.

## Sandboxing

Backends run in [landrun](https://github.com/zouuup/landrun) sandboxes. **Cannot be disabled** — it's the only safety layer.

**Defaults:**
- Filesystem: CWD, `/tmp`, config dirs (rw); `/usr`, `/lib`, `/bin` (ro+exec)
- Network: disabled
- Mode: strict (`best_effort: false`) — fails if sandbox unavailable

### Configuration

Sandbox configuration is stored in `~/.config/anvillm/sandbox.yaml`. Edit this file to customize sandbox behavior.

Path templates: `{CWD}`, `{HOME}`, `{TMPDIR}`

```yaml
# Enable network
network:
  enabled: true
  unrestricted: true

# Add paths
filesystem:
  rw:
    - "{CWD}"
    - "{HOME}/.npm"
```

Changes apply to new sessions. Use `Restart` command to reload configuration for existing sessions.

### Kernel Requirements

| Kernel | Landlock ABI | Features |
|--------|--------------|----------|
| 5.13+ | v1 | Filesystem |
| 6.7+ | v4 | Improved FS |
| 6.10+ | v5 | Network |

### Best-Effort Mode

Set `best_effort: true` for graceful degradation on older kernels.

**⚠️ Warning:** If landrun is missing or kernel lacks Landlock, sessions run **completely unsandboxed**.

## Reproducible Workflows

Script multi-agent workflows via 9P filesystem.

### Skills System

Extend agent capabilities on-demand:

```sh
anvillm-skills list                        # List available skills
anvillm-skills load anvillm-communication  # Load communication skill
```

See `kiro-cli/SKILLS_PROMPT.md` for details.

### Example: Developer-Reviewer

Two-agent code review workflow:

```sh
./workflows/DevReview claude /path/to/project
```

Developer implements and stages changes, sends review request via mailbox. Reviewer examines diff, sends feedback or "LGTM". Loop until approved.

### Example: Planning

Three-agent documentation workflow:

```sh
./workflows/Planning kiro /path/to/docs
```

**Research** queries codebase, **Engineering** writes docs (queries research when needed), **Tech-editor** reviews and requests changes.

### Writing Your Own

Key patterns:

```sh
# Create session, capture ID
echo "new claude /project" | 9p write agent/ctl
ID=$(9p read agent/list | tail -1 | awk '{print $1}')

# Set context (defines agent behavior)
cat <<EOF | 9p write agent/$ID/context
You are a code reviewer. When you receive code...
EOF

# Agents communicate via mailboxes
cat > /tmp/msg.json <<'EOF'
{"to":"$PEER_ID","type":"REVIEW_REQUEST","subject":"Review","body":"Please review staged changes"}
EOF
9p write agent/$ID/mail < /tmp/msg.json

# Track work with beads
echo 'new "Review PR #123"' | 9p write agent/beads/ctl
BEAD_ID=$(9p read agent/beads/list | jq -r '.[-1].id')
echo "claim $BEAD_ID" | 9p write agent/beads/ctl
```

See `workflows/DevReview` and `workflows/Planning` for complete examples.

## Troubleshooting

| Problem | Solution |
|---------|----------|
| "Failed to connect to anvilsrv" | Check `anvilsrv status`; try `anvilsrv start` |
| Session won't start | Check stderr; verify `which claude` or `which kiro-cli` |
| Landlock ABI error | Set `best_effort: true` or upgrade kernel |
| Permission denied | Add paths to sandbox config |
| Orphaned tmux | `tmux kill-session -t anvillm-0` (or anvillm-N for namespace :N) |
| 9P not working | Verify plan9port: `9p ls agent` |
| Stale PID file | `anvilsrv stop` cleans up automatically |
| Daemon won't stop | Check logs with `anvilsrv fgstart` |
| anvilweb can't connect | Ensure anvilsrv is running; check namespace matches |
| anvilweb port in use | Use `-addr :PORT` to specify different port |

**Debugging:** Run `anvilsrv fgstart` for foreground logs.

**Terminal:** `Attach` uses `$ANVILLM_TERMINAL` (defaults to `foot`).
