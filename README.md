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
mk  # installs to $HOME/bin
```

**Optional service integration:**

```sh
# systemd (user service, recommended)
cp services/systemd/anvilsrv-user.service ~/.config/systemd/user/
systemctl --user enable --now anvilsrv

# systemd (system service)
sudo cp services/systemd/anvilsrv.service /etc/systemd/system/
sudo systemctl enable --now anvilsrv

# runit
sudo cp -r services/runit /etc/sv/anvilsrv
sudo ln -s /etc/sv/anvilsrv /var/service/
```

See `services/*/README.md` for details.

## Usage

**Start server:**
```sh
anvilsrv start       # background
anvilsrv fgstart     # foreground (debugging)
anvilsrv status
anvilsrv stop
```

`Assist` auto-starts `anvilsrv` if not running.

### Multiple Namespaces

Run independent instances via `$NAMESPACE` (like `acme -a`). Default: `/tmp/ns.$USER.:0`
```sh
anvilsrv start                            # :0
NAMESPACE=/tmp/ns.$USER.:1 anvilsrv start  # :1
NAMESPACE=/tmp/ns.$USER.:1 Assist          # connect to :1
```

### Clients

**Terminal UI:**
```sh
anvillm
```
Keys: `s` start | `p` prompt | `t` stop | `R` restart | `K` kill | `a` alias | `r` refresh | `d` daemon | `?` help | `q` quit  
Navigation: arrows, vim (`j`/`k`), emacs (`C-n`/`C-p`)  
Attach: `tmux attach`

**Emacs:**
```elisp
(add-to-list 'load-path "/path/to/anvillm.el/")
(require 'anvillm)
```
Usage: `M-x anvillm`  
Keys: `s` start | `p` prompt | `P` prompt (minibuffer) | `t` stop | `R` restart | `K` kill | `a` alias | `r`/`g` refresh | `d` daemon | `q` quit | `?` help

**Web:**
```sh
anvilweb              # :8080
anvilweb -addr :3000  # custom port
```
⚠️ **NO authentication** — localhost/trusted networks only. For remote: `ssh -L 8080:localhost:8080 user@remote`

**Acme:**  
Type `Assist` and middle-click.

### Acme Commands

Tag: `Get Attach Stop Restart Kill Alias Context Log Daemon Inbox Archive`

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
| `Inbox [id]` | View inbox (default: user) |
| `Archive [id]` | View archive (default: user) |

Right-click session ID for prompt window. 2-1 chord on ID for fire-and-forget prompts.

## 9P Filesystem

Sessions exposed at `$NAMESPACE/agent`:

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

Path validation: `/../../../etc` → `/etc`, rejects nonexistent dirs.

### Beads Task Tracking

Persistent, crash-resilient tasks via [beads](https://github.com/steveyegge/beads). Dependency-aware graph for multi-agent workflows.

```sh
# Initialize
echo 'init' | 9p write agent/beads/ctl

# Create, claim, complete
echo 'new "Implement login" "Add JWT auth"' | 9p write agent/beads/ctl
echo 'claim bd-a1b2' | 9p write agent/beads/ctl
echo 'complete bd-a1b2' | 9p write agent/beads/ctl

# Dependencies (child blocks parent)
echo 'dep bd-child bd-parent' | 9p write agent/beads/ctl

# List ready tasks
9p read agent/beads/ready | jq -r '.[] | "\(.id): \(.title)"'
```

Config: `~/.beads/` (override: `ANVILLM_BEADS_PATH`). Shared across namespaces. See `internal/beads/README.md`.

### Events & Mailbox

**Events** (`/agent/events`):
```sh
9p read agent/events
# {"type":"state_change","session_id":"a3f2b9d1","state":"running",...}
```

**Mailbox** (inter-session messaging):
```sh
# Send
cat > /tmp/msg.json <<'EOF'
{"to":"a3f2b9d1","type":"REVIEW_REQUEST","subject":"Review PR","body":"..."}
EOF
9p write agent/b4e3c8f2/mail < /tmp/msg.json

# Read
9p read agent/a3f2b9d1/inbox
9p read agent/a3f2b9d1/archive
```

### Basic Usage

```sh
echo 'new claude /home/user/project' | 9p write agent/ctl
echo 'Hello' | 9p write agent/a3f2b9d1/in
9p read agent/a3f2b9d1/state  # "running"
echo 'stop' | 9p write agent/a3f2b9d1/ctl
```

See `SECURITY.md` for authentication details.

## Backends & Sandboxing

**Backends:**
- Claude: `npm install -g @anthropic-ai/claude-code`
- Kiro: [kiro.dev](https://kiro.dev)

Both run with full permissions (`--dangerously-skip-permissions`, `--trust-all-tools`).

**Add backends:** Implement `CommandHandler`/`StateInspector` in `internal/backends/yourbackend.go`, register in `main.go`.

**Sandboxing:** [landrun](https://github.com/zouuup/landrun) sandboxes (cannot disable). Defaults: CWD/`/tmp`/config (rw), `/usr`/`/lib`/`/bin` (ro+exec), no network, strict mode.

**Config** (`~/.config/anvillm/sandbox.yaml`):

```yaml
network:
  enabled: true
  unrestricted: true
filesystem:
  rw:
    - "{CWD}"
    - "{HOME}/.npm"
```

Templates: `{CWD}`, `{HOME}`, `{TMPDIR}`. Changes apply to new sessions; use `Restart` to reload.

**Kernel requirements:**
- 5.13+: Landlock v1 (filesystem)
- 6.7+: v4 (improved FS)
- 6.10+: v5 (network)

**Best-effort mode:** Set `best_effort: true` for older kernels. ⚠️ Runs **unsandboxed** if landrun missing or no Landlock.

**Self-healing:** Auto-restarts crashed sessions every 5s (preserves context/alias/cwd). Rate-limited, skips intentional stops. Check: `anvilsrv fgstart`

## Workflows

Script multi-agent workflows via 9P.

**Skills:**
```sh
anvillm-skills list
anvillm-skills load anvillm-communication
```

**Examples:**

*Developer-Reviewer* (two agents):
```sh
./workflows/DevReview claude /path/to/project
```
Developer implements/stages, sends review request. Reviewer examines diff, sends feedback/"LGTM". Loop until approved.

*Planning* (three agents):
```sh
./workflows/Planning kiro /path/to/docs
```
Research queries codebase, Engineering writes docs (queries research), Tech-editor reviews/requests changes.

**Writing workflows:**

```sh
# Create session
echo "new claude /project" | 9p write agent/ctl
ID=$(9p read agent/list | tail -1 | awk '{print $1}')

# Set context
echo "You are a code reviewer..." | 9p write agent/$ID/context

# Send message
echo '{"to":"$PEER","type":"REVIEW_REQUEST","subject":"Review","body":"..."}' | 9p write agent/$ID/mail

# Track with beads
echo 'new "Review PR #123"' | 9p write agent/beads/ctl
echo "claim $(9p read agent/beads/list | jq -r '.[-1].id')" | 9p write agent/beads/ctl
```

See `workflows/DevReview` and `workflows/Planning`. Details: `kiro-cli/SKILLS_PROMPT.md`.

## Troubleshooting

| Problem | Solution |
|---------|----------|
| Can't connect | `anvilsrv status`; try `anvilsrv start` |
| Session won't start | Check stderr; verify backend installed |
| Landlock ABI error | Set `best_effort: true` or upgrade kernel |
| Permission denied | Add paths to `sandbox.yaml` |
| Orphaned tmux | `tmux kill-session -t anvillm-0` |
| 9P not working | `9p ls agent` |
| Stale PID | `anvilsrv stop` auto-cleans |
| Daemon won't stop | `anvilsrv fgstart` for logs |
| anvilweb issues | Check anvilsrv running, namespace matches |

Debug: `anvilsrv fgstart`  
Terminal: `$ANVILLM_TERMINAL` (default: `foot`)
