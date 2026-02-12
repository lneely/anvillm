// Package tmux provides tmux-based backend implementation for CLI tools
package tmux

import (
	"anvillm/internal/backend"
	"anvillm/internal/debug"
	"anvillm/internal/sandbox"
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"syscall"
	"time"
)

// TmuxSize defines terminal dimensions
type TmuxSize struct {
	Rows uint16
	Cols uint16
}

// StartupHandler can send initial commands during startup
type StartupHandler interface {
	// HandleStartup is called during waitForReady when output is received
	// Returns tmux key sequence to send (e.g., "Down", "C-m"), or empty string if no action needed
	// Returns done=true when startup is complete
	HandleStartup(output string) (keys string, done bool)
}

// StateInspector checks if the backend is busy by inspecting the process tree
type StateInspector interface {
	// IsBusy checks if the backend has running child processes (tools, etc.)
	// Takes the pane PID from tmux
	IsBusy(panePID int) bool
}

// Config holds tmux backend configuration
type Config struct {
	Name           string
	Command        []string
	Environment    map[string]string
	TmuxSize       TmuxSize
	StartupTime    time.Duration
	Commands       backend.CommandHandler
	StartupHandler StartupHandler // Optional: handles startup dialogs
	StateInspector StateInspector // Optional: for process tree inspection
}

// Backend implements backend.Backend for tmux-based CLI tools
type Backend struct {
	cfg         Config
	tmuxSession string // Persistent tmux session name (e.g., "anvillm-kiro-cli")
}

// generateID creates a unique session ID using random bytes
func generateID() string {
	b := make([]byte, 4) // 8 hex characters
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

// New creates a new tmux-based backend
func New(cfg Config) backend.Backend {
	if cfg.StartupTime == 0 {
		cfg.StartupTime = 30 * time.Second
	}
	if cfg.TmuxSize.Rows == 0 {
		cfg.TmuxSize.Rows = 40
	}
	if cfg.TmuxSize.Cols == 0 {
		cfg.TmuxSize.Cols = 120
	}

	return &Backend{
		cfg:         cfg,
		tmuxSession: fmt.Sprintf("anvillm-%s", cfg.Name),
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

func (b *Backend) CreateSession(ctx context.Context, cwd string) (backend.Session, error) {
	id := generateID()
	windowName := id // Use session ID as window name

	debug.Log("[session %s] creating window in tmux session %s", id, b.tmuxSession)

	// Load sandbox configuration fresh for each session
	sandboxCfg, err := sandbox.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading sandbox config: %v (using defaults)\n", err)
		debug.Log("[session %s] failed to load sandbox config: %v, using defaults", id, err)
		sandboxCfg = sandbox.DefaultConfig()
	}

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
	for k, v := range b.cfg.Environment {
		if err := setEnvironment(target, k, v); err != nil {
			killWindow(b.tmuxSession, windowName)
			return nil, fmt.Errorf("failed to set environment: %w", err)
		}
	}

	// 4. Create FIFO for output
	fifoPath := fmt.Sprintf("/tmp/tmux-%s-%s.fifo", b.tmuxSession, windowName)
	// Remove existing FIFO if it exists (from previous crashed session)
	os.Remove(fifoPath)
	if err := syscall.Mkfifo(fifoPath, 0600); err != nil {
		killWindow(b.tmuxSession, windowName)
		return nil, fmt.Errorf("failed to create FIFO: %w", err)
	}

	// 5. Setup pipe-pane for this window
	if err := setupPipePane(target, fifoPath); err != nil {
		os.Remove(fifoPath)
		killWindow(b.tmuxSession, windowName)
		return nil, fmt.Errorf("failed to setup pipe-pane: %w", err)
	}

	// 6. Open FIFO for reading (this blocks until writer connects)
	// We need to do this in a goroutine to avoid blocking
	fifoOpenCh := make(chan *os.File, 1)
	fifoErrCh := make(chan error, 1)
	go func() {
		f, err := os.OpenFile(fifoPath, os.O_RDONLY, 0600)
		if err != nil {
			fifoErrCh <- err
			return
		}
		fifoOpenCh <- f
	}()

	// 7. Wrap command with landrun (ALWAYS - sandboxing cannot be disabled)
	command := b.cfg.Command
	if !sandbox.IsAvailable() {
		if sandboxCfg.General.BestEffort {
			fmt.Fprintf(os.Stderr, "WARNING: landrun not available, running UNSANDBOXED (best-effort mode)\n")
			fmt.Fprintf(os.Stderr, "WARNING: No filesystem or network restrictions will be enforced!\n")
			fmt.Fprintf(os.Stderr, "Install landrun: go install github.com/landlock-lsm/landrun@latest\n")
		} else {
			os.Remove(fifoPath)
			killWindow(b.tmuxSession, windowName)
			fmt.Fprintf(os.Stderr, "ERROR: landrun not available and sandboxing is required\n")
			fmt.Fprintf(os.Stderr, "Install landrun: go install github.com/landlock-lsm/landrun@latest\n")
			fmt.Fprintf(os.Stderr, "Or enable best_effort mode in ~/.config/anvillm/sandbox.yaml (NOT RECOMMENDED)\n")
			return nil, fmt.Errorf("landrun not available")
		}
	}
	command = sandbox.WrapCommand(sandboxCfg, command, cwd)
	debug.Log("[session %s] sandboxed command: %v", id, command)

	// 8. Build command string for tmux
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
	if err := sendKeys(target, fmt.Sprintf("cd \"%s\"", cwd), "C-m"); err != nil {
		os.Remove(fifoPath)
		killWindow(b.tmuxSession, windowName)
		return nil, fmt.Errorf("failed to change directory: %w", err)
	}

	// Send the command
	if err := sendKeys(target, cmdStr, "C-m"); err != nil {
		os.Remove(fifoPath)
		killWindow(b.tmuxSession, windowName)
		return nil, fmt.Errorf("failed to start command: %w", err)
	}

	// 8. Wait for FIFO to open (should happen quickly after command starts)
	var fifo *os.File
	select {
	case fifo = <-fifoOpenCh:
		// Success
	case err := <-fifoErrCh:
		os.Remove(fifoPath)
		killWindow(b.tmuxSession, windowName)
		return nil, fmt.Errorf("failed to open FIFO: %w", err)
	case <-time.After(5 * time.Second):
		os.Remove(fifoPath)
		killWindow(b.tmuxSession, windowName)
		return nil, fmt.Errorf("timeout opening FIFO")
	}

	// 9. Get PID
	pid, err := getPanePID(target)
	if err != nil {
		debug.Log("[session %s] warning: failed to get PID: %v", id, err)
		pid = 0
	}

	sess := &Session{
		id:             id,
		backendName:    b.cfg.Name,
		tmuxSession:    b.tmuxSession,
		windowName:     windowName,
		cwd:            cwd,
		pid:            pid,
		state:          "starting",
		createdAt:      time.Now(),
		fifoPath:       fifoPath,
		fifo:           fifo,
		dataCh:         make(chan []byte, 100),
		stopCh:         make(chan struct{}),
		commands:       b.cfg.Commands,
		startupHandler: b.cfg.StartupHandler,
		stateInspector: b.cfg.StateInspector,
	}

	// 10. Start reader goroutine
	go sess.reader()

	// 11. Wait for ready
	if err := sess.waitForReady(ctx, b.cfg.StartupTime); err != nil {
		sess.Close()
		return nil, err
	}

	sess.state = "idle"
	sess.output.Reset()
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
