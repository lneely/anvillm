# AnviLLM

Acme-native interface for LLM chat backends. Sessions appear as Acme windows and are exposed via 9P filesystem.

## Requirements

- Go 1.21+, plan9port, tmux
- [landrun](https://github.com/landlock-lsm/landrun) (sandboxing, Linux kernel 5.13+)
- Backend CLI: [Claude Code](https://github.com/anthropics/claude-code) or [kiro-cli](https://github.com/stillmatic/kiro)

## Installation

```sh
git clone https://github.com/lneely/anvillm
cd anvillm
mk  # installs to $HOME/bin
```

## Usage

Type `Assist` in Acme and middle-click to open the `/AnviLLM/` session manager.

### Commands

**Main window** (`/AnviLLM/`) tag: `Get Attach Kill Alias Context Sandbox`

| Command | Description |
|---------|-------------|
| `Kiro <dir>` | Start kiro-cli session |
| `Claude <dir>` | Start Claude session |
| `Kill <id>` | Terminate session |
| `Attach <id>` | Open tmux terminal |
| `Alias <id> <name>` | Name a session |
| `Context <id>` | Edit context (prepended to prompts) |
| `Sandbox` | Configure sandboxing |

Right-click a session ID to open its prompt window. Select text anywhere and 2-1 chord on a session ID for fire-and-forget prompts.

**Prompt window** (`+Prompt.<id>`) tag: `Send`

Type prompt, click `Send`.

### Notifications

Desktop notifications on session completion via `anvillm-notify` (starts automatically, requires `notify-send`).

## 9P Filesystem

```
agent/
├── ctl             # "new <backend> <cwd>" creates session
├── list            # id, alias, state, pid, cwd
└── <id>/
    ├── in          # Write prompts
    ├── out         # Read responses
    ├── ctl         # "stop" or "kill"
    ├── state       # idle, running, exited
    ├── context     # Prepended to prompts (r/w)
    ├── alias       # Session name (r/w)
    ├── pid, cwd, winid, backend
    └── err         # (unused)
```

Example:
```sh
echo 'new claude /home/user/project' > agent/ctl
echo 'Hello' > agent/a3f2b9d1/in
9p read agent/a3f2b9d1/state
```

## Backends

| Backend | Install | Notes |
|---------|---------|-------|
| Claude | `npm install -g @anthropic-ai/claude-code` | Auto-saves to `~/.claude/projects/` |
| Kiro | [kiro-cli](https://github.com/stillmatic/kiro) | `/chat save` and `/chat load` |

Both run with full permissions inside the sandbox (`--dangerously-skip-permissions`, `--trust-all-tools`).

### Adding Backends

Create `internal/backends/yourbackend.go`, implement `CommandHandler` and `StateInspector` interfaces, register in `main.go`. See existing backends for examples.

## Sandboxing

Backends run inside [landrun](https://github.com/landlock-lsm/landrun) sandboxes. **Sandboxing cannot be disabled** — it's the only safety layer.

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

## Troubleshooting

| Problem | Solution |
|---------|----------|
| Session won't start | Check stderr; verify `which claude` or `which kiro-cli` |
| Landlock ABI error | Set `best_effort: true` or upgrade kernel |
| Permission denied | Add paths to sandbox config |
| Orphaned tmux | `tmux kill-session -t anvillm-claude` |
| 9P not working | Verify plan9port: `9p ls agent` |

Terminal for `Attach` configured in `main.go` (`terminalCommand`).
