// Package tmux provides tmux-based backend implementation for CLI tools
package tmux

import (
	"acme-q/internal/backend"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"sync/atomic"
	"syscall"
	"time"
)

// TmuxSize defines terminal dimensions
type TmuxSize struct {
	Rows uint16
	Cols uint16
}

// Detector handles pattern matching for backend output
type Detector interface {
	// IsReady checks if backend is ready for first prompt
	IsReady(output string) bool

	// IsComplete checks if response to given prompt is complete
	IsComplete(prompt string, output string) bool
}

// Cleaner processes raw output from the backend
type Cleaner interface {
	// Clean removes noise (ANSI codes, spinners, etc.) and extracts actual content
	Clean(prompt string, rawOutput string) string
}

// StartupHandler can send initial commands during startup
type StartupHandler interface {
	// HandleStartup is called during waitForReady when output is received
	// Returns tmux key sequence to send (e.g., "Down", "C-m"), or empty string if no action needed
	// Returns done=true when startup is complete
	HandleStartup(output string) (keys string, done bool)
}

// Config holds tmux backend configuration
type Config struct {
	Name           string
	Command        []string
	Environment    map[string]string
	TmuxSize       TmuxSize
	StartupTime    time.Duration
	Detector       Detector
	Cleaner        Cleaner
	Commands       backend.CommandHandler
	StartupHandler StartupHandler // Optional: handles startup dialogs
}

// Backend implements backend.Backend for tmux-based CLI tools
type Backend struct {
	cfg     Config
	counter uint64
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
	return &Backend{cfg: cfg}
}

func (b *Backend) Name() string {
	return b.cfg.Name
}

func (b *Backend) CreateSession(ctx context.Context, cwd string) (backend.Session, error) {
	id := fmt.Sprintf("%d", atomic.AddUint64(&b.counter, 1))

	// Generate unique session name with random suffix
	randomBytes := make([]byte, 4)
	rand.Read(randomBytes)
	sessionName := fmt.Sprintf("%s-%d-%s",
		b.cfg.Name,
		time.Now().Unix(),
		hex.EncodeToString(randomBytes))

	log.Printf("[session %s] creating tmux session %s", id, sessionName)

	// 1. Create tmux session
	if err := createSession(sessionName, b.cfg.TmuxSize.Rows, b.cfg.TmuxSize.Cols); err != nil {
		return nil, fmt.Errorf("failed to create tmux session: %w", err)
	}

	// 2. Set environment variables
	for k, v := range b.cfg.Environment {
		if err := setEnvironment(sessionName, k, v); err != nil {
			killSession(sessionName)
			return nil, fmt.Errorf("failed to set environment: %w", err)
		}
	}

	// 3. Create FIFO for output
	fifoPath := fmt.Sprintf("/tmp/tmux-%s.fifo", sessionName)
	if err := syscall.Mkfifo(fifoPath, 0600); err != nil {
		killSession(sessionName)
		return nil, fmt.Errorf("failed to create FIFO: %w", err)
	}

	// 4. Setup pipe-pane
	if err := setupPipePane(sessionName, fifoPath); err != nil {
		os.Remove(fifoPath)
		killSession(sessionName)
		return nil, fmt.Errorf("failed to setup pipe-pane: %w", err)
	}

	// 5. Open FIFO for reading (this blocks until writer connects)
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

	// 6. Start the command in tmux
	cmdStr := ""
	for i, arg := range b.cfg.Command {
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
	if err := sendKeys(sessionName, "cd", fmt.Sprintf("\"%s\"", cwd), "C-m"); err != nil {
		os.Remove(fifoPath)
		killSession(sessionName)
		return nil, fmt.Errorf("failed to change directory: %w", err)
	}

	// Send the command
	if err := sendKeys(sessionName, cmdStr, "C-m"); err != nil {
		os.Remove(fifoPath)
		killSession(sessionName)
		return nil, fmt.Errorf("failed to start command: %w", err)
	}

	// 7. Wait for FIFO to open (should happen quickly after command starts)
	var fifo *os.File
	select {
	case fifo = <-fifoOpenCh:
		// Success
	case err := <-fifoErrCh:
		os.Remove(fifoPath)
		killSession(sessionName)
		return nil, fmt.Errorf("failed to open FIFO: %w", err)
	case <-time.After(5 * time.Second):
		os.Remove(fifoPath)
		killSession(sessionName)
		return nil, fmt.Errorf("timeout opening FIFO")
	}

	// 8. Get PID
	pid, err := getSessionPID(sessionName)
	if err != nil {
		log.Printf("[session %s] warning: failed to get PID: %v", id, err)
		pid = 0
	}

	sess := &Session{
		id:             id,
		sessionName:    sessionName,
		cwd:            cwd,
		pid:            pid,
		state:          "starting",
		createdAt:      time.Now(),
		fifoPath:       fifoPath,
		fifo:           fifo,
		dataCh:         make(chan []byte, 100),
		stopCh:         make(chan struct{}),
		detector:       b.cfg.Detector,
		cleaner:        b.cfg.Cleaner,
		commands:       b.cfg.Commands,
		startupHandler: b.cfg.StartupHandler,
	}

	// 9. Start reader goroutine
	go sess.reader()

	// 10. Wait for ready
	if err := sess.waitForReady(ctx, b.cfg.StartupTime); err != nil {
		sess.Close()
		return nil, err
	}

	sess.state = "idle"
	sess.output.Reset()
	log.Printf("[session %s] ready (tmux=%s, pid=%d)", sess.ID(), sessionName, sess.pid)

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
