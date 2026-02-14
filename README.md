# AnviLLM

9P-based LLM orchestrator. Front-ends for Acme, Emacs, TUI, and web.

## Architecture

AnviLLM consists of:

- **anvilsrv**: Background daemon managing sessions via 9P
- **Assist**: Acme UI client (auto-starts daemon if needed)
- **anvillm.el**: For our emacs friends ;)
- **anvillm**: Terminal UI (curses-based) for everyone else
- **anvilweb**: Web interface for browser-based access

This separation allows:
- Multiple clients to connect to the same sessions
- Server survives client crashes/restarts
- Scriptable workflows via 9P without UI dependencies
- Service management (systemd/runit integration)
- Choice of interface: Acme, terminal, Emacs, or web browser

## Requirements

- Go 1.21+, plan9port, tmux
- [landrun](https://github.com/zouuup/landrun) (sandboxing, Linux kernel 5.13+)
- Backend CLI: [Claude Code](https://github.com/anthropics/claude-code) or [Kiro CLI](https://kiro.dev)

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

`anvilsrv` respects the `$NAMESPACE` environment variable, allowing multiple independent instances for different Acme sessions (similar to running multiple Acme instances with `acme -a`).

**Default namespace**: `/tmp/ns.$USER.:0`

**Example: Running multiple instances**
```sh
# Start default instance (namespace :0)
anvilsrv start

# Start second instance (namespace :1)
NAMESPACE=/tmp/ns.$USER.:1 anvilsrv start

# Check status of each instance
anvilsrv status                           # :0
NAMESPACE=/tmp/ns.$USER.:1 anvilsrv status  # :1

# Each instance has isolated:
# - PID file: $NAMESPACE/anvilsrv.pid
# - 9P socket: $NAMESPACE/agent
# - Session list (completely independent)

# Connect Assist to specific instance
NAMESPACE=/tmp/ns.$USER.:1 Assist
```

This enables running separate AnviLLM environments for different projects or Acme workspaces.

### Terminal UI (anvillm)

Run `anvillm` for a curses-based interface (no Acme required):

```sh
anvillm
```

**Keyboard shortcuts:**
- `s` - Start new session (shows backend menu)
- `p` - Send prompt to selected session
- `t` - Stop selected session
- `R` - Restart selected session
- `K` - Kill selected session
- `a` - Set session alias
- `r` - Refresh session list
- `d` - Daemon status
- `?` - Help
- `q` - Quit

To attach to the anvillm session, simply open a different terminal and use `tmux attach`.

**Navigation:**
- Arrow keys (`↑`/`↓`)
- Vim-style (`j`/`k`)
- Emacs-style (`C-n`/`C-p`)

Like Assist, the TUI reads/writes the 9P filesystem for all operations.

### Emacs Interface (anvillm.el)

Run `M-x anvillm` in Emacs for a native Emacs interface:

**Installation:**

Copy anvillm.el to ~/.emacs.d/lisp/ or wherever your elisp files live. Then, add to your emacs init:

```elisp
(add-to-list 'load-path "/path/to/anvillm.el/")
(require 'anvillm)
```

**Keybindings:**

- s - Start new session (select backend)
- p - Send prompt to selected session
- P - Send prompt to selected session using minibuffer
- t - Stop selected session
- R - Restart selected session
- K - Kill selected session
- a - Set alias for selected session
- r - Refresh session list
- g - Refresh session list (standard Emacs binding)
- d - Daemon status
- q - Quit
- ? - Help

**Navigation:**
- Standard Emacs navigation (`n`, `p`, `C-n`, `C-p`)
- Sessions displayed in a sortable table (click column headers)

The Emacs interface uses the `9p` command from plan9port to interact with the filesystem.

### Web Interface (anvilweb)

Run `anvilweb` for a browser-based interface:

```sh
anvilweb              # starts on :8080
anvilweb -addr :3000  # custom port
```

**Features:**
- Start new sessions (Kiro/Claude)
- View all sessions with live state updates
- Send prompts, edit context and aliases
- Stop/restart/kill sessions
- Auto-refreshes every 5 seconds

**Security Warning:**

⚠️ **The web interface has NO authentication.** Anyone who can reach the server port can:
- View all sessions
- Send prompts to any session
- Start/stop/kill sessions
- Modify session contexts

**Only run anvilweb on localhost or trusted networks.** For remote access, use SSH port forwarding:

```sh
# On remote machine
anvilweb -addr localhost:8080

# On local machine
ssh -L 8080:localhost:8080 user@remote
# Then browse to http://localhost:8080
```

Or use a reverse proxy with authentication (nginx, caddy, etc.).

### Acme Interface (Assist)

Type `Assist` in Acme and middle-click to open the `/AnviLLM/` session manager.

### Commands

**Main window** (`/AnviLLM/`) tag: `Get Attach Stop Restart Kill Refresh Alias Context Daemon Sandbox`

| Command | Description |
|---------|-------------|
| `Kiro <dir>` | Start kiro-cli session |
| `Claude <dir>` | Start Claude session |
| `Stop <id>` | Stop session (preserves tmux) |
| `Restart <id>` | Restart stopped session |
| `Kill <id>` | Terminate session |
| `Refresh <id>` | Re-detect session state |
| `Attach <id>` | Open tmux terminal |
| `Alias <id> <name>` | Name a session |
| `Context <id>` | Edit context (prepended to prompts) |
| `Daemon` | Manage anvilsrv daemon (start/stop/status) |
| `Sandbox` | Configure sandbox settings |

Right-click a session ID to open its prompt window. Select text anywhere and 2-1 chord on a session ID for fire-and-forget prompts.

**Note**: Assist reconnects to the running `anvilsrv` daemon, so sessions persist across Assist restarts.

**Prompt window** (`+Prompt.<id>`) tag: `Send`

Type prompt, click `Send`.

## 9P Filesystem

The `anvilsrv` daemon exposes sessions via a 9P filesystem at `$NAMESPACE/agent` (typically `/tmp/ns.$USER/agent`).

```
agent/
├── ctl             # "new <backend> <cwd>" creates session
├── list            # id, alias, state, pid, cwd
└── <id>/
    ├── in          # Write prompts
    ├── out         # Read responses
    ├── ctl         # "stop", "restart", "kill", "refresh"
    ├── state       # starting, idle, running, stopped, error, exited
    ├── context     # Prepended to prompts (r/w)
    ├── alias       # Session name (r/w)
    ├── pid         # Process ID
    ├── cwd         # Working directory
    └── backend     # Backend name
```

**Path Validation**: Session creation validates and cleans paths (e.g., `/../../../etc` → `/etc`), and rejects nonexistent directories.

Example:
```sh
echo 'new claude /home/user/project' | 9p write agent/ctl
echo 'Hello' | 9p write agent/a3f2b9d1/in
9p read agent/a3f2b9d1/state  # → "running"
echo 'stop' | 9p write agent/a3f2b9d1/ctl
9p read agent/a3f2b9d1/state  # → "stopped"
```

**Security**: See `SECURITY.md` for 9P socket authentication limitations.

## Backends

| Backend | Install |
|---------|---------|
| Claude | `npm install -g @anthropic-ai/claude-code` |
| Kiro | [Kiro CLI](https://kiro.dev) |

Both run with full permissions inside the sandbox (`--dangerously-skip-permissions`, `--trust-all-tools`).

### Adding Backends

Create `internal/backends/yourbackend.go`, implement `CommandHandler` and `StateInspector` interfaces, register in `main.go`. See existing backends for examples.

## Self-Healing

The `anvilsrv` daemon monitors all sessions every 5 seconds and automatically restarts sessions that crash unexpectedly. This ensures long-running workflows remain resilient to backend failures.

**Behavior:**
- Detects when a session process terminates without explicit `Stop` command
- Automatically restarts the session in the same working directory
- Preserves session context and alias
- Rate-limited to prevent restart loops (minimum 5 seconds between attempts)
- Does not restart sessions stopped intentionally via `Stop` command

Sessions that crash are transparently restarted without user intervention. Check logs with `anvilsrv fgstart` to see auto-restart activity.

## Sandboxing

Backends run inside [landrun](https://github.com/zouuup/landrun) sandboxes. **Sandboxing cannot be disabled** — it's the only safety layer.

### Defaults

- Filesystem: CWD, `/tmp`, config dirs (rw); `/usr`, `/lib`, `/bin` (ro+exec)
- Network: disabled
- Mode: strict (`best_effort: false`) — sessions fail if sandbox unavailable

### Configuration

Click `Sandbox` in main window, then `Edit` to modify `~/.config/anvillm/sandbox.yaml`.

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

Changes apply to new sessions only.

### Kernel Requirements

| Kernel | Landlock ABI | Features |
|--------|--------------|----------|
| 5.13+ | v1 | Filesystem |
| 6.7+ | v4 | Improved FS |
| 6.10+ | v5 | Network |

### Best-Effort Mode

Set `best_effort: true` for graceful degradation on older kernels.

**Warning**: If landrun is missing or kernel lacks Landlock, sessions run **completely unsandboxed**. Only enable if you accept this risk.

## Reproducible Workflows

The 9P filesystem enables scripted multi-agent workflows. Create sessions, set contexts, and wire agents together programmatically.

### Example: Developer-Reviewer

Two agents collaborate on code changes — one implements, one reviews:

```sh
./scripts/DevReview claude /path/to/project
```

Creates paired sessions with contexts that instruct agents to:
1. Developer implements and stages changes
2. Developer sends review request via `agent/{reviewer-id}/in`
3. Reviewer examines diff, sends feedback or "LGTM"
4. Loop until approved

### Example: Planning

Three-agent workflow for documentation tasks:

```sh
./scripts/Planning kiro /path/to/docs
```

- **Research**: Queries codebase and knowledge base
- **Engineering**: Writes/updates documentation, queries research when needed
- **Tech-editor**: Reviews for quality, requests changes from engineering

### Writing Your Own

Key patterns from the example scripts:

```sh
# Create session, capture ID
echo "new claude /project" | 9p write agent/ctl
ID=$(9p read agent/list | tail -1 | awk '{print $1}')

# Set context (defines agent behavior)
cat <<EOF | 9p write agent/$ID/context
You are a code reviewer. When you receive code...
EOF

# Agents communicate via each other's in files
echo "Please review staged changes" | 9p write agent/$PEER_ID/in
```

See `scripts/DevReview` and `scripts/Planning` for complete examples.

## Troubleshooting

| Problem | Solution |
|---------|----------|
| "Failed to connect to anvilsrv" | Check `anvilsrv status`; try `anvilsrv start` |
| Session won't start | Check stderr; verify `which claude` or `which kiro-cli` |
| Landlock ABI error | Set `best_effort: true` or upgrade kernel |
| Permission denied | Add paths to sandbox config |
| Orphaned tmux | `tmux kill-session -t anvillm-claude` |
| 9P not working | Verify plan9port: `9p ls agent` |
| Stale PID file | `anvilsrv stop` cleans up automatically |
| Daemon won't stop | Check logs with `anvilsrv fgstart` |
| anvilweb can't connect | Ensure anvilsrv is running; check namespace matches |
| anvilweb port in use | Use `-addr :PORT` to specify different port |

**Debugging**: Run `anvilsrv fgstart` to see daemon logs in foreground.

**Terminal**: `Attach` command uses `$ANVILLM_TERMINAL` or defaults to `foot`. Configure in environment or see `cmd/Assist/main.go`.
