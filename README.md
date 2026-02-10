# AnviLLM

**An Acme-native interface for LLM chat backends**

AnviLLM brings Claude, Kiro, and other LLM chat interfaces directly into your Acme editor. It provides:

- **Native Acme integration**: Chat sessions appear as Acme windows with familiar tag commands
- **9P filesystem**: Sessions exposed as files (`9p ls agent`)
- **Multi-backend support**: Run sessions for Claude and Kiro (and other supported backends) simultaneously
- **Tmux isolation**: Each session runs in its own tmux window for direct access when needed
- **Session management**: Save/load transcripts (if supported by backend, otherwise take advantage of acme), alias sessions, switch between multiple conversations .
- **If all else fails**: Fire up a terminal and attach to the correct `tmux` session with a single click.

## Architecture

```
┌─────────────────────────────────────────────────┐
│                 Acme Windows                    │
│  ┌──────────────┐  ┌──────────────┐            │
│  │ /AnviLLM/    │  │ /chat/claude │            │
│  │ Sessions     │  │ Save Load    │            │
│  └──────────────┘  └──────────────┘            │
└─────────────────────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────┐
│              Session Manager                    │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐     │
│  │ Backend: │  │ Backend: │  │ Backend: │     │
│  │  Claude  │  │ Kiro-CLI │  │  (more)  │     │
│  └──────────┘  └──────────┘  └──────────┘     │
└─────────────────────────────────────────────────┘
                       │
         ┌─────────────┴─────────────┐
         ▼                           ▼
┌──────────────────┐      ┌──────────────────────┐
│  9P Filesystem   │      │   Tmux Sessions      │
│  /mnt/anvillm/   │      │  anvillm-claude      │
│   agent/         │      │  anvillm-kiro-cli    │
│     <id>/        │      └──────────────────────┘
│       in         │
│       out        │
│       ctl        │
│       backend    │
└──────────────────┘
```

## Requirements

- **Go 1.21+** (for building)
- **Plan 9 from User Space** (plan9port) - provides Acme and 9P tools
- **tmux** - session isolation and management
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

Type `Assist` somewhere in Acme, and middle click. This opens the main `/AnviLLM/` window in Acme showing all active sessions (probably empty).

### Creating Sessions

From the `/AnviLLM/` window, use tag commands:

- **Kiro** `<directory>` - Start a kiro-cli session in the specified directory
- **Claude** `<directory>` - Start a Claude Code session in the specified directory

Example: Click on a path, and 2-1 chord on `Claude` to start a Claude session.

Each session opens a new `$PWD/+Chat.<session-id>` window where you can:
- Type prompts in the window body
- Use `Get` to send the text as a prompt
- Use `Save` to export the transcript
- Use `Load` to import a previous transcript
- Use `Attach` to open the underlying tmux window

### Session Management Commands

#### Main Window (`/AnviLLM/`)
- **Open** `<session-id>` - Reopen a session window (2-1 chord with session ID highlighted)
- **Kill** `<pid>` - Terminate a session (2-1 chord with pid highlighted)

#### Chat Windows (`/chat/<backend>/<id>`)
- **Send** - Send the window body as a prompt to the LLM
- **Save** `[path]` - Save transcript to file (if backend supportrs it, defaults to `~/.config/anvillm/transcripts/<timestamp>.txt`)
- **Load** `<path>` - Load a previous transcript (if backend supports it)
- **Alias** `<name>` - Give the session a memorable name (2-1 click with a `name` argument)
- **Attach** - Open the tmux window for direct terminal access
- **Stop** - Interrupt the current running prompt (CTRL+C)

## 9P Filesystem

AnviLLM exports a 9P filesystem at `/mnt/anvillm`:

```
/mnt/anvillm/
└── agent/
    ├── ctl                 # Control file (create new sessions)
    ├── list                # list of active sessions
    └── <session-id>/
        ├── in           # Write prompts here
        ├── out         # Read responses here
        ├── err        # Error output
        ├── ctl         # Session control (stop, close)
        ├── state       # Current state (idle, running, exited)
        ├── pid         # Process ID
        ├── cwd         # Working directory
        ├── alias       # Session alias
        ├── winid       # Acme window ID
        └── backend     # Backend name (claude, kiro-cli)
```

### 9P Usage Examples

Create a new session:
```sh
echo 'new claude /home/user/project' > agent/ctl
```

Send a prompt:
```sh
echo 'What is the capital of France?' > agent/a3f2b9d1/in
cat /mnt/anvillm/agent/a3f2b9d1/out
```

Check session state:
```sh
cat agent/a3f2b9d1/state
# Output: idle
```

## Backends

### Supported Backends

#### Claude (Claude Code CLI)
- Official Anthropic CLI for Claude
- Supports chat, session save/load, slash commands
- Install: `npm install -g @anthropic-ai/claude-code`

#### Kiro-CLI
- Alternative Claude interface provided by Amazon for AWS development
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
        StartupTime: 15 * time.Second,
        Detector:    &yourDetector{},
        Cleaner:     &yourCleaner{},
        Commands:    &yourCommandHandler{},
    })
}
```

2. Implement the required interfaces:
   - `Detector` - Detect when the backend is ready and when responses complete
   - `Cleaner` - Strip ANSI codes and noise from output
   - `CommandHandler` - Handle backend-specific slash commands

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

## Troubleshooting

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
