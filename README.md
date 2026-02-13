# AnviLLM

**An Acme-native interface for LLM chat backends**

AnviLLM brings Claude, Kiro, and other LLM chat interfaces directly into your Acme editor. It provides:

- **Native Acme integration**: Chat sessions appear as Acme windows with familiar tag commands
- **9P filesystem**: Sessions exposed as files (`9p ls agent`)
- **Multi-backend support**: Run sessions for Claude and Kiro (and other supported backends) simultaneously
- **Tmux isolation**: Each session runs in its own tmux window for direct access when needed
- **Session management**: Alias sessions, set context, switch between multiple conversations
- **If all else fails**: Fire up a terminal and attach to the correct `tmux` session with a single click.

## Architecture

```
┌─────────────────────────────────────────────────┐
│                 Acme Windows                    │
│  ┌──────────────┐  ┌──────────────┐             │
│  │ /AnviLLM/    │  │ +Prompt.abc1 │             │
│  │ Sessions     │  │ Send         │             │
│  └──────────────┘  └──────────────┘             │
└─────────────────────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────┐
│              Session Manager                    │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐       │
│  │ Backend: │  │ Backend: │  │ Backend: │       │
│  │  Claude  │  │ Kiro-CLI │  │  (more)  │       │
│  └──────────┘  └──────────┘  └──────────┘       │
└─────────────────────────────────────────────────┘
                       │
         ┌─────────────┴─────────────┐
         ▼                           ▼
┌──────────────────┐      ┌──────────────────────┐
│  9P Filesystem   │      │   Tmux Sessions      │
│   agent/         │      │  anvillm-claude      │
│     ctl          │      │  anvillm-kiro-cli    │
│     list         │      └──────────────────────┘
│     <id>/        │
│       in         │
│       out        │
│       ctl        │
│       context    │
│       backend    │
└──────────────────┘
```

## Requirements

- **Go 1.21+** (for building)
- **Plan 9 from User Space** (plan9port) - provides Acme and 9P tools
- **tmux** - session isolation and management
- **landrun** - sandboxing tool (optional but recommended)
  - Install: `go install github.com/landlock-lsm/landrun@latest`
  - Requires Linux kernel 5.13+ with Landlock support
  - See Sandboxing section below for kernel version details
- **At least one backend CLI**:
  - [Claude Code CLI](https://github.com/anthropics/claude-code) - Anthropic's official CLI
  - [kiro-cli](https://github.com/stillmatic/kiro) - Alternative Claude interface

## Installation

```sh
git clone https://github.com/lneely/anvillm
cd anvillm
mk  # installs to $HOME/bin
```

## Usage

### Starting AnviLLM

Type `Assist` somewhere in Acme and middle-click. This opens the main `/AnviLLM/` window showing all active sessions.

### Creating Sessions

From the `/AnviLLM/` window, use tag commands:

- **Kiro** `<directory>` - Start a kiro-cli session in the specified directory
- **Claude** `<directory>` - Start a Claude Code session in the specified directory

Example: Click on a path, then 2-1 chord on `Claude` to start a Claude session.

Each session opens a `$PWD/+Prompt.<session-id>` window where you can:
- Type prompts in the window body
- Use `Send` to submit the prompt to the LLM

### Session Management Commands

#### Main Window (`/AnviLLM/`)

Tag: `Get Attach Kill Alias Context Sandbox`

- **Get** - Refresh the session list
- **Kill** `<session-id>` - Terminate a session (2-1 chord with session ID)
- **Attach** `<session-id>` - Open the tmux window for direct terminal access
- **Alias** `<session-id> <name>` - Give a session a memorable name
- **Context** `<session-id>` - Open context editor window (text prepended to every prompt)
- **Sandbox** - Open sandbox configuration window

Right-click a session ID to reopen its prompt window.

#### Prompt Windows (`+Prompt.<id>`)

Tag: `Send`

- **Send** - Send the window body as a prompt to the LLM

#### Fire-and-Forget Prompts

Select text anywhere in Acme, then 2-1 chord on a session ID (the 8-character hex ID shown in `/AnviLLM/`). The selected text is sent as a prompt to that session without switching windows.

### Notifications

The `anvillm-notify` script monitors sessions and sends desktop notifications when a session transitions from running to idle.

```sh
anvillm-notify &
```

Requires `notify-send` (libnotify). Starts automatically with AnviLLM.

## 9P Filesystem

AnviLLM exports a 9P filesystem at `agent`:

```
agent/
├── ctl             # Control file (create new sessions)
├── list            # List of active sessions
└── <session-id>/
    ├── in          # Write prompts here
    ├── out         # Read responses here
    ├── err         # Error output (currently unused)
    ├── ctl         # Session control (stop, kill)
    ├── state       # Current state (idle, running, exited)
    ├── pid         # Process ID
    ├── cwd         # Working directory
    ├── alias       # Session alias (r/w)
    ├── winid       # Acme window ID (r/w)
    ├── context     # Text prepended to every prompt (r/w)
    └── backend     # Backend name (claude, kiro-cli)
```

### 9P Usage Examples

Create a new session:
```sh
echo 'new claude /home/user/project' > agent/ctl
```

List sessions:
```sh
9p read agent/list
# Output: a3f2b9d1  -  idle  12345  /home/user/project
```

Send a prompt:
```sh
echo 'What is the capital of France?' > agent/a3f2b9d1/in
9p read agent/a3f2b9d1/out
```

Check session state:
```sh
9p read agent/a3f2b9d1/state
# Output: idle
```

Set context for bot-to-bot communication:
```sh
echo 'You are dev. Peer reviewer is at agent/abc123' > agent/a3f2b9d1/context
```

## Backends

### Supported Backends

#### Claude (Claude Code CLI)
- Official Anthropic CLI for Claude
- Runs with `--dangerously-skip-permissions` (sandboxing via landrun)
- Auto-saves conversations to `~/.claude/projects/<dir-path>/<session-id>.jsonl`
- Install: `npm install -g @anthropic-ai/claude-code`

#### Kiro-CLI
- AWS CLI interface for Kiro
- Runs with `--trust-all-tools` (sandboxing via landrun)
- Supports `/chat load` and `/chat save` for session persistence
- Install: See [kiro-cli repository](https://github.com/stillmatic/kiro)

### Adding New Backends

Backends are defined in `internal/backends/`. To add a new backend:

1. Create a new function in `internal/backends/yourbackend.go`:

```go
func NewYourBackend() backend.Backend {
    return tmux.New(tmux.Config{
        Name:    "yourbackend",
        Command: []string{"your-cli-command", "args"},
        Environment: map[string]string{
            "TERM":     "xterm-256color",
            "NO_COLOR": "1",
        },
        TmuxSize: tmux.TmuxSize{
            Rows: 40,
            Cols: 120,
        },
        StartupTime:    15 * time.Second,
        Commands:       &yourCommandHandler{},
        StateInspector: &yourStateInspector{},
    })
}
```

2. Implement the required interfaces:
   - `CommandHandler` - Handle backend-specific slash commands
   - `StateInspector` - Check if backend is busy via process tree inspection

3. Register in `main.go`:

```go
backendMap := map[string]backend.Backend{
    "kiro-cli":    backends.NewKiroCLI(),
    "claude":      backends.NewClaude(),
    "yourbackend": backends.NewYourBackend(),
}
```

4. Add tag command in `main.go` event loop

See `internal/backends/kirocli.go` or `claude.go` for complete examples.

## Configuration

### Terminal Command

The terminal used for `Attach` is configured in `main.go`:

```go
const terminalCommand = "foot"  // Change to: "kitty", "xterm", "alacritty", etc.
```

### Tmux Session Names

Backends create persistent tmux sessions named `anvillm-<backend>`:
- `anvillm-claude`
- `anvillm-kiro-cli`

These sessions persist across AnviLLM restarts and are cleaned up on exit.

## Sandboxing

AnviLLM uses [landrun](https://github.com/landlock-lsm/landrun) to sandbox backend processes for enhanced security. Sandboxing restricts filesystem and network access for LLM-controlled processes while allowing backends to run with full permissions (`--dangerously-skip-permissions`, `--trust-all-tools`) within the sandbox.

### How It Works

```
landrun --rw /project --rox /usr --connect-tcp 443 -- claude --dangerously-skip-permissions
         ↑                                                   ↑
         Sandbox layer (restricts access)                   Backend runs with full permissions
                                                             (but only within sandbox)
```

### Quick Start

Sandboxing is **always enabled** and cannot be disabled. The default policy:
- **Filesystem**: Read/write access to session working directory, temp dir, and config dirs only
- **System binaries**: Read-only + execute access to `/usr`, `/lib`, `/bin`, `/sbin`
- **System files**: Read-only access to essential files (`/etc/passwd`, `/dev/null`, `/proc/meminfo`, etc.)
- **Network**: Disabled by default (enable in config if needed)
- **Config files**: Access to `~/.claude`, `~/.claude.json`, `~/.kiro`, `~/.config/anvillm`

**Security philosophy**:
AnviLLM runs backends with `--dangerously-skip-permissions` and `--trust-all-tools`, giving LLMs full access within their sandbox. **The sandbox is the ONLY safety layer.** Therefore, sandboxing cannot be disabled.

**Important defaults**:
- **Sandboxing: ALWAYS ENABLED** - No config option to disable
- **`best_effort: false`** (strict mode - RECOMMENDED): Sessions **fail** if sandboxing cannot be applied
  - Prevents accidentally running with ZERO restrictions when:
    - `landrun` is not installed or not in PATH
    - Kernel doesn't support Landlock (< 5.13)
  - Security-first: Explicit failure is safer than silent degradation to completely unsandboxed mode
  - **Do NOT change to `true`** unless you understand the security implications (see Troubleshooting below)
- **`network.enabled: false`**: Network disabled by default; enable and configure ports as needed

### Requirements

- **landrun**: Install with `go install github.com/landlock-lsm/landrun@latest`
- **Linux Kernel with Landlock**:
  - **Kernel 5.13+** (Landlock ABI v1): Basic filesystem restrictions
  - **Kernel 6.7+** (Landlock ABI v4): Improved filesystem control
  - **Kernel 6.10+** (Landlock ABI v5): Network restrictions (TCP port control)

### Modes

- **Strict mode** (default, `best_effort: false`):
  - Sessions **fail** if sandboxing cannot be applied
  - Prevents accidentally running unsandboxed
  - Security-first: Explicit failure rather than silent degradation
- **Best-effort mode** (`best_effort: true`):
  - Gracefully degrades to older Landlock ABI versions if needed
  - **CRITICAL**: If `landrun` binary not in PATH → command runs without landrun wrapper (ZERO restrictions)
  - **CRITICAL**: If kernel lacks Landlock (< 5.13) → landrun wrapper present but ineffective (ZERO restrictions)
  - Prints warning to stderr but no error returned - session starts successfully
  - Recommended for kernels < 6.10 when you need ABI v5 features but want graceful degradation

**Security implications of best-effort mode**:
- Two failure modes that result in **completely unsandboxed** execution:
  1. **`landrun` binary missing**: Backend runs directly without sandbox wrapper
  2. **Kernel lacks Landlock**: Backend runs with landrun wrapper that cannot enforce restrictions
- Either case: No filesystem isolation, no network restrictions - full system access
- Use only when you understand and accept this tradeoff

**Note**: Default config uses unrestricted network access (works on any kernel). Fine-grained network restrictions (port-specific) require ABI v5 (kernel 6.10+).

### Configuration

Configure sandboxing via the `Sandbox` tag command in the main `/AnviLLM/` window:

1. Click **Sandbox** to open the configuration window
2. Click **Edit** to modify `~/.config/anvillm/sandbox.yaml`
3. Click **Reload** to reload configuration from disk
4. Create new sessions to apply changes

Configuration file: `~/.config/anvillm/sandbox.yaml`

**Important**: Changes apply to NEW sessions only. Active sessions are NOT affected.

### Path Templates

Use these templates in filesystem paths:
- `{CWD}` - Session working directory
- `{HOME}` - User home directory (`~`)
- `{TMPDIR}` - Temporary directory (`/tmp`)

### Common Configurations

#### Development (Network Enabled)
```yaml
network:
  enabled: true
  unrestricted: true

filesystem:
  rw:
    - "{CWD}"
    - "{HOME}/.npm"
    - "{HOME}/.cache"
    - "{HOME}/.local"
  rox:
    - "/usr"
    - "/lib"
    - "/bin"
```

#### Code Review (Restrictive)
```yaml
network:
  enabled: false

filesystem:
  ro:
    - "{CWD}"  # Read-only project access
  rw:
    - "{TMPDIR}"
  rox:
    - "/usr"
    - "/bin"
```

### Security Model

**Default protections**:
- ✓ Cannot read sensitive files outside CWD (e.g., `~/.ssh/id_rsa`, `~/.aws/credentials`)
- ✓ Cannot modify system files
- ✓ Cannot make unauthorized network connections
- ✓ Cannot access files in other users' directories

**User can still**:
- Modify files in session CWD (by design - project workspace)
- Weaken sandboxing via config (user choice)

### Troubleshooting

**Session fails to launch with "missing kernel Landlock support" or ABI version error?**

This happens when landrun requires a newer Landlock ABI than your kernel provides. Check your kernel version:
```sh
uname -r
# Kernel 6.8 = Landlock ABI v4
# Kernel 6.10+ = Landlock ABI v5
```

**Why this happens**: By default, AnviLLM uses **strict mode** (`best_effort: false`). Sessions fail if sandboxing cannot be fully applied, rather than silently falling back to unsandboxed execution. This is intentional for security.

**Solution 1: Enable best-effort mode** (recommended for kernel < 6.10)

Edit `~/.config/anvillm/sandbox.yaml`:
```yaml
general:
  best_effort: true  # Gracefully degrade if ABI too old
```

This allows sandboxing to gracefully degrade when network restrictions aren't available (requires ABI v5). Filesystem restrictions will still be enforced on kernels with Landlock support (ABI v1+, kernel 5.13+).

**CRITICAL SECURITY WARNING**: With `best_effort: true`:
- If `landrun` binary is not in PATH → runs with **ZERO restrictions** (completely unsandboxed)
  - AnviLLM omits landrun from the command entirely
  - Backend runs directly: `claude --dangerously-skip-permissions` (no sandbox wrapper)
  - Warning printed to stderr: "landrun not available, running unsandboxed"
- If `landrun` IS in PATH but kernel lacks Landlock support (< 5.13) → runs with **ZERO restrictions** (completely unsandboxed)
  - AnviLLM wraps command: `landrun --best-effort ... -- claude ...`
  - landrun executes but internally uses go-landlock library in BestEffort mode
  - go-landlock silently succeeds without applying any restrictions (no error from library)
  - Backend runs with landrun wrapper present but ineffective
- If ABI version too old (e.g., kernel 6.8 with v4 when v5 needed) → degrades gracefully
  - landrun runs with available ABI version (filesystem restrictions still enforced)
  - Network restrictions skipped if not supported by kernel
  - Partial sandboxing maintained

Only enable best-effort mode if you understand and accept the possibility of running completely unsandboxed.

**Solution 2: Upgrade your kernel**

Install kernel 6.10+ for full Landlock ABI v5 support with network restrictions.

**Solution 3: Disable network restrictions** (already default)

The default config uses `unrestricted: true` for network access, which works on older kernels.

**Landrun not available?**
- Install: `go install github.com/landlock-lsm/landrun@latest`
- Verify: `which landrun` should show the path (e.g., `/home/user/go/bin/landrun`)
- Check status: Click `Sandbox` → `Status` in AnviLLM
- **Default behavior** (`best_effort: false`): Sessions **fail** if landrun binary is missing (secure)
  - Error printed to stderr: "sandboxing enabled but landrun not available"
  - Session creation aborted - no backend process started
- **With** `best_effort: true`: Backend runs **without landrun wrapper** if binary is missing (DANGEROUS)
  - landrun completely omitted from command
  - Backend executes directly: `claude --dangerously-skip-permissions`
  - No filesystem isolation - full read/write access to entire system
  - No network restrictions - unrestricted network access
  - Warning printed to stderr but session starts successfully

**Recommendation**: Keep `best_effort: false` unless you have a specific need for graceful degradation and understand the security implications.

**Backend fails with permission denied errors?**

Node.js-based backends (Claude, Kiro) require specific system files. The default config includes these, but if you've modified it, ensure you have:

```yaml
filesystem:
  ro:
    - /etc/passwd       # Required: Node.js UID→homedir lookup
    - /dev/null         # Required: null device
    - /proc/meminfo     # Required: memory info
    - /proc/self/cgroup # Required: cgroup info
    - /proc/self/maps   # Required: process memory maps
    - /proc/version     # Required: kernel version
  rw:
    - '{HOME}/.claude.json'  # Claude config file
```

**File access denied?**
- Add required paths to `rw` or `ro` in config
- Example for SSH keys: `ro: ["{HOME}/.ssh"]`
- Example for NPM: `rw: ["{HOME}/.npm", "{HOME}/.cache"]`

**Check sandbox status**:
```sh
# View current config
cat ~/.config/anvillm/sandbox.yaml

# Test landrun availability
landrun --version

# Check kernel Landlock ABI version
dmesg | grep -i landlock
```

**Error messages**:
Launch errors are printed to stderr regardless of debug mode. If a session fails to start, check the error output for specific issues.

For more details, see [landrun documentation](https://github.com/landlock-lsm/landrun).

## Troubleshooting

### Session fails to start

If sessions fail to launch, error messages are printed to stderr. Common causes:
- **Sandboxing issues**: See Sandboxing → Troubleshooting section above
- **Backend not installed**: Run `which claude` or `which kiro-cli`
- **Landlock ABI mismatch**: Enable `best_effort: true` in sandbox config (see above)

### "Backend not found" error

Make sure the backend CLI is installed and in your PATH:
```sh
which claude
which kiro-cli
```

### Tmux windows not cleaned up

If AnviLLM exits unexpectedly, you may have orphaned tmux sessions:
```sh
tmux list-sessions | grep anvillm
tmux kill-session -t anvillm-claude
tmux kill-session -t anvillm-kiro-cli
```

### Can't connect to 9P filesystem

Make sure plan9port is installed and 9P tools are working:
```sh
9p ls agent
```
