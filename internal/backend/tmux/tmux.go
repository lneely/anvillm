// Package tmux provides tmux-based backend implementation for CLI tools
package tmux

import (
	"anvillm/internal/backend"
	"anvillm/internal/debug"
	"anvillm/internal/logging"
	"anvillm/pkg/sandbox"
	"context"
	"crypto/rand"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// TmuxSize defines terminal dimensions
type TmuxSize struct {
	Rows uint16
	Cols uint16
}

// StateInspector checks if the backend is busy by inspecting the process tree
type StateInspector interface {
	// IsBusy checks if the backend has running child processes (tools, etc.)
	// Takes the pane PID from tmux
	IsBusy(panePID int) bool
}

// ModelResolver modifies a base command to include a model selection flag or argument.
// It receives the base command slice and the model name, and returns the modified command.
// For flag-based CLIs: append(cmd, "--model", model)
// For positional CLIs: replace the relevant positional argument
type ModelResolver func(cmd []string, model string) []string

// ClearHandler handles the /clear command for a specific backend.
// It receives the tmux target and should send the appropriate keys.
// Returns nil on success.
type ClearHandler func(target string) error

// ResumeHandler handles the resume command for a specific backend.
// It receives the tmux target and should send the appropriate keys.
// Returns nil on success.
type ResumeHandler func(target string) error

// CompactHandler handles the /compact command for a specific backend.
// It receives the tmux target and should send the appropriate keys.
// Returns nil on success.
type CompactHandler func(target string) error

// Config holds tmux backend configuration
type Config struct {
	Name           string
	Command        []string
	Environment    map[string]string
	TmuxSize       TmuxSize
	Commands       backend.CommandHandler
	StateInspector StateInspector // Optional: for process tree inspection
	NsSuffix       string         // Optional: namespace suffix (e.g., "0" for :0)
	ModelResolver  ModelResolver  // Optional: modifies command to include model selection
	ClearHandler   ClearHandler   // Optional: backend-specific /clear handling
	ResumeHandler  ResumeHandler  // Optional: backend-specific resume handling
	CompactHandler CompactHandler // Optional: backend-specific /compact handling
}

// Backend implements backend.Backend for tmux-based CLI tools
type Backend struct {
	cfg         Config
	tmuxSession string // Persistent tmux session name (e.g., "anvillm-0")
}

// generateID creates a unique session ID using random bytes
func generateID() string {
	b := make([]byte, 4) // 8 hex characters
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

// New creates a new tmux-based backend
func New(cfg Config) backend.Backend {
	if cfg.TmuxSize.Rows == 0 {
		cfg.TmuxSize.Rows = 40
	}
	if cfg.TmuxSize.Cols == 0 {
		cfg.TmuxSize.Cols = 120
	}

	// Build tmux session name with namespace suffix if provided
	// All backends share the same tmux session per namespace
	sessionName := "anvillm"
	if cfg.NsSuffix != "" {
		sessionName = fmt.Sprintf("%s-%s", sessionName, cfg.NsSuffix)
	}

	return &Backend{
		cfg:         cfg,
		tmuxSession: sessionName,
	}
}

// ensureTmuxSession creates the persistent tmux session if it doesn't exist
func (b *Backend) ensureTmuxSession() error {
	if sessionExists(b.tmuxSession) {
		return nil
	}

	debug.Log("[backend %s] creating persistent tmux session: %s", b.cfg.Name, b.tmuxSession)
	// Keep window 0 alive to hold the session open
	// It will be killed when the program exits
	return createSession(b.tmuxSession, b.cfg.TmuxSize.Rows, b.cfg.TmuxSize.Cols)
}

// Cleanup kills the persistent tmux session (call on program exit)
func (b *Backend) Cleanup() error {
	if sessionExists(b.tmuxSession) {
		debug.Log("[backend %s] cleaning up tmux session: %s", b.cfg.Name, b.tmuxSession)
		return killSession(b.tmuxSession)
	}
	return nil
}

func (b *Backend) Name() string {
	return b.cfg.Name
}

func (b *Backend) CreateSession(ctx context.Context, opts backend.SessionOptions) (backend.Session, error) {
	id := generateID()
	windowName := id // Use session ID as window name

	debug.Log("[session %s] creating window in tmux session %s (sandbox=%s)", id, b.tmuxSession, opts.Sandbox)

	// Build layered sandbox configuration
	// Start with global.yaml as base
	baseCfg, err := sandbox.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load global config: %w", err)
	}

	// Convert base config to layered format
	baseLayer := sandbox.LayeredConfig{
		Filesystem: baseCfg.Filesystem,
		Network:    baseCfg.Network,
		Env:        baseCfg.Env,
	}
	layers := []sandbox.LayeredConfig{baseLayer}

	backendLayer, err := sandbox.LoadBackend(b.cfg.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to load backend config %q: %w", b.cfg.Name, err)
	}
	layers = append(layers, backendLayer)

	// Add sandbox layer if specified, otherwise use "default"
	sbx := opts.Sandbox
	if sbx == "" {
		sbx = sandbox.DefaultSandbox
	}
	sbxLayer, err := sandbox.LoadSandbox(sbx)
	if err != nil {
		return nil, fmt.Errorf("failed to load sandbox %q: %w", sbx, err)
	}
	layers = append(layers, sbxLayer)

	// Merge layers into final config
	general := sandbox.GeneralConfig{
		BestEffort: false,
		LogLevel:   "error",
	}
	advanced := sandbox.AdvancedConfig{
		LDD:     false,
		AddExec: true,
	}
	sandboxCfg := sandbox.Merge(general, advanced, layers...)

	// 1. Ensure persistent tmux session exists
	if err := b.ensureTmuxSession(); err != nil {
		return nil, fmt.Errorf("failed to ensure tmux session: %w", err)
	}

	// 2. Create window in tmux session
	if err := createWindow(b.tmuxSession, windowName); err != nil {
		return nil, fmt.Errorf("failed to create window: %w", err)
	}

	// Give shell time to initialize (login scripts, etc.)
	time.Sleep(500 * time.Millisecond)

	target := windowTarget(b.tmuxSession, windowName)

	// 3. Set environment variables for this window
	// Set AGENT_ID first
	if err := setEnvironment(target, "AGENT_ID", id); err != nil {
		killWindow(b.tmuxSession, windowName)
		return nil, fmt.Errorf("failed to set AGENT_ID: %w", err)
	}

	// Set recovery metadata as window options (survives daemon restart)
	setWindowOption(target, "ANVILLM_BACKEND", b.cfg.Name)
	setWindowOption(target, "ANVILLM_CWD", opts.CWD)
	if opts.Sandbox != "" {
		setWindowOption(target, "ANVILLM_SANDBOX", opts.Sandbox)
	}
	if opts.Model != "" {
		setWindowOption(target, "ANVILLM_MODEL", opts.Model)
	}

	for k, v := range b.cfg.Environment {
		if err := setEnvironment(target, k, v); err != nil {
			killWindow(b.tmuxSession, windowName)
			return nil, fmt.Errorf("failed to set environment: %w", err)
		}
		if err := sendKeys(target, fmt.Sprintf("export %s=%s", k, v), "C-m"); err != nil {
			killWindow(b.tmuxSession, windowName)
			return nil, fmt.Errorf("failed to export environment: %w", err)
		}
	}

	// 4. Wrap command with landrun (ALWAYS - sandboxing cannot be disabled)
	command := b.cfg.Command
	// Apply model resolver if a model is specified
	if opts.Model != "" && b.cfg.ModelResolver != nil {
		command = b.cfg.ModelResolver(command, opts.Model)
	}
	if !sandbox.IsAvailable() {
		if sandboxCfg.General.BestEffort {
			logging.Logger().Warn("landrun not available, running UNSANDBOXED (best-effort mode)")
			logging.Logger().Warn("no filesystem and network restrictions will be enforced")
			logging.Logger().Warn("install landrun: go install github.com/landlock-lsm/landrun@latest")
		} else {
			killWindow(b.tmuxSession, windowName)
			logging.Logger().Error("landrun not available and sandboxing is required")
			logging.Logger().Error("install landrun: go install github.com/landlock-lsm/landrun@latest")
			logging.Logger().Error("or enable best_effort mode in ~/.config/anvillm/global.yaml (NOT RECOMMENDED)")
			return nil, fmt.Errorf("landrun not available")
		}
	}
	command = sandbox.WrapCommand(sandboxCfg, command, opts.CWD)
	debug.Log("[session %s] sandboxed command: %v", id, command)

	// 5. Build command string for tmux
	cmdStr := ""
	for i, arg := range command {
		if i > 0 {
			cmdStr += " "
		}
		// Quote arguments with spaces
		if containsSpace(arg) {
			cmdStr += fmt.Sprintf("\"%s\"", arg)
		} else {
			cmdStr += arg
		}
	}

	// Change to working directory first
	if err := sendKeys(target, fmt.Sprintf("cd \"%s\"", opts.CWD), "C-m"); err != nil {
		killWindow(b.tmuxSession, windowName)
		return nil, fmt.Errorf("failed to change directory: %w", err)
	}

	// Export AGENT_ID to shell environment
	if err := sendKeys(target, fmt.Sprintf("export AGENT_ID=%s", id), "C-m"); err != nil {
		killWindow(b.tmuxSession, windowName)
		return nil, fmt.Errorf("failed to export AGENT_ID: %w", err)
	}

	// Send the command
	if err := sendKeys(target, cmdStr, "C-m"); err != nil {
		killWindow(b.tmuxSession, windowName)
		return nil, fmt.Errorf("failed to start command: %w", err)
	}

	// 6. Get PID (find the backend process, not just the bash shell)
	panePID, err := getPanePID(target)
	if err != nil {
		debug.Log("[session %s] warning: failed to get pane PID: %v", id, err)
		panePID = 0
	}

	// Find the actual backend process (child of bash shell)
	pid := 0
	if panePID != 0 {
		// For kiro-cli, the pane process IS kiro-cli (no bash in between)
		// Check if pane is kiro-cli directly
		cmd := exec.Command("ps", "-p", fmt.Sprintf("%d", panePID), "-o", "comm=")
		if out, err := cmd.Output(); err == nil {
			comm := strings.TrimSpace(string(out))
			if comm == "kiro-cli" {
				pid = panePID
				debug.Log("[session %s] pane IS kiro-cli (PID: %d)", id, pid)
			}
		}

		// Otherwise, try to find backend as child of bash
		if pid == 0 {
			for i := 0; i < 10; i++ {
				time.Sleep(200 * time.Millisecond)
				pid = FindBackendPID(panePID)
				if pid != 0 {
					debug.Log("[session %s] found backend PID %d (pane PID: %d)", id, pid, panePID)
					break
				}
			}
		}
		if pid == 0 {
			debug.Log("[session %s] warning: backend process not found for pane PID %d", id, panePID)
		}
	}

	sess := &Session{
		id:             id,
		backendName:    b.cfg.Name,
		tmuxSession:    b.tmuxSession,
		windowName:     windowName,
		cwd:            opts.CWD,
		sandbox:        sbx,
		model:          opts.Model,
		modelResolver:  b.cfg.ModelResolver,
		clearHandler:   b.cfg.ClearHandler,
		resumeHandler:  b.cfg.ResumeHandler,
		compactHandler: b.cfg.CompactHandler,
		pid:            pid,
		state:          "idle",
		createdAt:      time.Now(),
		idleSince:      time.Now(),
		stopCh:         make(chan struct{}),
		commands:       b.cfg.Commands,
		stateInspector: b.cfg.StateInspector,
		// Store for restart support
		backendCommand:     b.cfg.Command,
		environment:        b.cfg.Environment,
		originalCommandStr: cmdStr,
	}
	sess.idleCond = sync.NewCond(&sess.mu)

	debug.Log("[session %s] ready (tmux=%s:%s, pid=%d)", sess.ID(), b.tmuxSession, windowName, sess.pid)
	return sess, nil
}

func containsSpace(s string) bool {
	for _, r := range s {
		if r == ' ' || r == '\t' {
			return true
		}
	}
	return false
}

// ListOrphanedWindows returns window names in the tmux session that look like session IDs
// (8 hex chars) but are not in the tracked set.
func (b *Backend) ListOrphanedWindows(tracked map[string]bool) []string {
	if !sessionExists(b.tmuxSession) {
		return nil
	}

	out, err := tmuxCmd("list-windows", "-t", b.tmuxSession, "-F", "#{window_name}")
	if err != nil {
		return nil
	}

	var orphans []string
	for _, name := range strings.Split(strings.TrimSpace(out), "\n") {
		if name == "" || name == "bash" {
			continue
		}
		// Check if it looks like a session ID (8 hex chars)
		if len(name) == 8 && isHex(name) && !tracked[name] {
			orphans = append(orphans, name)
		}
	}
	return orphans
}

// RecoverSession reconnects to a running tmux window.
// Returns a fully functional Session and the backend name.
func (b *Backend) RecoverSession(windowName string) (backend.Session, string, error) {
	if !windowExists(b.tmuxSession, windowName) {
		return nil, "", fmt.Errorf("window %s does not exist", windowName)
	}

	target := windowTarget(b.tmuxSession, windowName)

	// Read recovery metadata from window options
	envBackend := getWindowOption(target, "ANVILLM_BACKEND")
	envCwd := getWindowOption(target, "ANVILLM_CWD")
	envSandbox := getWindowOption(target, "ANVILLM_SANDBOX")
	envModel := getWindowOption(target, "ANVILLM_MODEL")
	envAlias := getWindowOption(target, "ANVILLM_ALIAS")
	envRole := getWindowOption(target, "ANVILLM_ROLE")

	// Use stored CWD if available, fallback to pane path
	cwd, _ := tmuxCmd("display-message", "-t", target, "-p", "#{pane_current_path}")
	cwd = strings.TrimSpace(cwd)
	if envCwd != "" {
		cwd = envCwd
	}

	// Get PID
	panePID, _ := getPanePID(target)
	pid := 0
	if panePID != 0 {
		cmd := exec.Command("ps", "-p", fmt.Sprintf("%d", panePID), "-o", "comm=")
		if out, err := cmd.Output(); err == nil {
			if strings.TrimSpace(string(out)) == "kiro-cli" {
				pid = panePID
			}
		}
		if pid == 0 {
			pid = FindBackendPID(panePID)
		}
	}

	// Use stored backend name, fallback to recovering backend
	backendName := envBackend
	if backendName == "" {
		backendName = b.cfg.Name
	}

	sess := &Session{
		id:             windowName,
		backendName:    backendName,
		tmuxSession:    b.tmuxSession,
		windowName:     windowName,
		cwd:            cwd,
		alias:          envAlias,
		sandbox:        envSandbox,
		model:          envModel,
		role:           envRole,
		pid:            pid,
		state:          "idle",
		createdAt:      time.Now(),
		stopCh:         make(chan struct{}),
		commands:       b.cfg.Commands,
		stateInspector: b.cfg.StateInspector,
		backendCommand: b.cfg.Command,
		environment:    b.cfg.Environment,
	}
	sess.idleCond = sync.NewCond(&sess.mu)

	// Detect actual state via process inspection
	if sess.stateInspector != nil {
		if panePID != 0 && sess.stateInspector.IsBusy(panePID) {
			sess.state = "running"
		}
	}

	debug.Log("[session %s] recovered (backend=%s, cwd=%s, pid=%d, state=%s)", windowName, backendName, cwd, pid, sess.state)
	return sess, backendName, nil
}

func isHex(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}
