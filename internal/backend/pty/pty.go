// Package pty provides PTY-based backend implementation for CLI tools
package pty

import (
	"anvillm/internal/debug"
	"anvillm/internal/backend"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/creack/pty"
)

// PTYSize defines terminal dimensions
type PTYSize struct {
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
	// Returns data to write to PTY, or nil if no action needed
	// Returns done=true when startup is complete
	HandleStartup(output string) (data []byte, done bool)
}

// Config holds PTY backend configuration
type Config struct {
	Name           string
	Command        []string
	Environment    map[string]string
	PTYSize        PTYSize
	StartupTime    time.Duration
	Detector       Detector
	Cleaner        Cleaner
	Commands       backend.CommandHandler
	StartupHandler StartupHandler // Optional: handles startup dialogs
}

// Backend implements backend.Backend for PTY-based CLI tools
type Backend struct {
	cfg     Config
	counter uint64
}

// New creates a new PTY-based backend
func New(cfg Config) backend.Backend {
	if cfg.StartupTime == 0 {
		cfg.StartupTime = 30 * time.Second
	}
	if cfg.PTYSize.Rows == 0 {
		cfg.PTYSize.Rows = 40
	}
	if cfg.PTYSize.Cols == 0 {
		cfg.PTYSize.Cols = 120
	}
	return &Backend{cfg: cfg}
}

func (b *Backend) Name() string {
	return b.cfg.Name
}

func (b *Backend) CreateSession(ctx context.Context, cwd string) (backend.Session, error) {
	id := fmt.Sprintf("%d", atomic.AddUint64(&b.counter, 1))

	cmd := exec.CommandContext(ctx, b.cfg.Command[0], b.cfg.Command[1:]...)
	cmd.Dir = cwd

	// Apply environment
	env := os.Environ()
	for k, v := range b.cfg.Environment {
		env = append(env, k+"="+v)
	}
	cmd.Env = env

	// Start with PTY
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Rows: b.cfg.PTYSize.Rows,
		Cols: b.cfg.PTYSize.Cols,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start pty: %w", err)
	}

	sess := &Session{
		id:             id,
		cwd:            cwd,
		pid:            cmd.Process.Pid,
		state:          "starting",
		createdAt:      time.Now(),
		cmd:            cmd,
		ptmx:           ptmx,
		dataCh:         make(chan []byte, 100),
		stopCh:         make(chan struct{}),
		detector:       b.cfg.Detector,
		cleaner:        b.cfg.Cleaner,
		commands:       b.cfg.Commands,
		startupHandler: b.cfg.StartupHandler,
	}

	// Start reader goroutine
	go sess.reader()

	// Wait for ready
	if err := sess.waitForReady(ctx, b.cfg.StartupTime); err != nil {
		sess.Close()
		return nil, err
	}

	sess.state = "idle"
	sess.output.Reset()
	debug.Log("[session %s] ready (pid=%d)", sess.ID(), sess.pid)

	return sess, nil
}

// Session implements backend.Session for PTY-based tools
type Session struct {
	id        string
	cwd       string
	alias     string
	winID     int
	pid       int
	state     string
	createdAt time.Time

	cmd    *exec.Cmd
	ptmx   *os.File
	dataCh chan []byte
	stopCh chan struct{}
	output bytes.Buffer

	detector       Detector
	cleaner        Cleaner
	commands       backend.CommandHandler
	startupHandler StartupHandler

	mu sync.Mutex
}

func (s *Session) ID() string {
	return s.id
}

func (s *Session) State() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

func (s *Session) SetAlias(alias string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.alias = alias
}

func (s *Session) Metadata() backend.SessionMetadata {
	s.mu.Lock()
	defer s.mu.Unlock()

	return backend.SessionMetadata{
		Pid:       s.pid,
		Cwd:       s.cwd,
		Alias:     s.alias,
		WinID:     s.winID,
		CreatedAt: s.createdAt,
	}
}

func (s *Session) Commands() backend.CommandHandler {
	return s.commands
}

// SetWinID sets the Acme window ID (called from main.go)
func (s *Session) SetWinID(winID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.winID = winID
}

// GetWinID gets the Acme window ID
func (s *Session) GetWinID() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.winID
}

// GetPid gets the process ID
func (s *Session) GetPid() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pid
}

// reader continuously reads from PTY and sends to dataCh
func (s *Session) reader() {
	buf := make([]byte, 4096)
	for {
		n, err := s.ptmx.Read(buf)
		if err != nil {
			if err != io.EOF {
				debug.Log("[session %s] reader error: %v", s.id, err)
			}
			close(s.dataCh)
			// Reap the process
			if s.cmd != nil && s.cmd.Process != nil {
				s.cmd.Wait()
			}
			s.mu.Lock()
			s.state = "exited"
			s.pid = 0
			s.mu.Unlock()
			return
		}
		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])
			s.dataCh <- data
		}
	}
}

func (s *Session) waitForReady(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	startupDone := s.startupHandler == nil // If no handler, startup is already "done"

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case data, ok := <-s.dataCh:
			if !ok {
				return fmt.Errorf("session closed")
			}
			s.output.Write(data)

			// If we have a startup handler and it hasn't completed, call it
			if !startupDone && s.startupHandler != nil {
				response, done := s.startupHandler.HandleStartup(s.output.String())
				if response != nil {
					if _, err := s.ptmx.Write(response); err != nil {
						return fmt.Errorf("failed to write startup response: %w", err)
					}
				}
				if done {
					startupDone = true
					s.output.Reset() // Reset output after startup handling
				}
			}

			// Check if ready (only after startup is done)
			if startupDone && s.detector.IsReady(s.output.String()) {
				return nil
			}
		case <-time.After(time.Until(deadline)):
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for ready")
			}
		}
	}
}

func (s *Session) Send(ctx context.Context, prompt string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.ptmx == nil {
		return "", fmt.Errorf("session not running")
	}

	if s.state == "starting" {
		return "", fmt.Errorf("session still starting")
	}

	// Check command support if applicable
	if strings.HasPrefix(prompt, "/") && s.commands != nil {
		if !s.commands.IsSupported(prompt) {
			return "", fmt.Errorf("unsupported command")
		}
	}

	// Reset stopCh for this request
	select {
	case <-s.stopCh:
		s.stopCh = make(chan struct{})
	default:
	}

	s.state = "running"
	s.output.Reset()

	debug.Log("[session %s] sending: %q", s.id, prompt)
	if _, err := s.ptmx.Write([]byte(prompt + "\r")); err != nil {
		s.state = "idle"
		return "", fmt.Errorf("write failed: %w", err)
	}

	// Wait for completion
	if err := s.waitForComplete(ctx, prompt); err != nil {
		s.state = "idle"
		return "", err
	}

	s.state = "idle"

	// Clean output
	result := s.cleaner.Clean(prompt, s.output.String())
	debug.Log("[session %s] response: %d bytes", s.id, len(result))

	return result, nil
}

func (s *Session) waitForComplete(ctx context.Context, prompt string) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.stopCh:
			return fmt.Errorf("interrupted")
		case data, ok := <-s.dataCh:
			if !ok {
				return fmt.Errorf("session closed")
			}
			s.output.Write(data)

			if s.detector.IsComplete(prompt, s.output.String()) {
				// Drain any remaining data
				for {
					select {
					case extra, ok := <-s.dataCh:
						if ok {
							s.output.Write(extra)
						}
					case <-time.After(10 * time.Millisecond):
						debug.Log("[session %s] response complete, %d bytes", s.id, s.output.Len())
						return nil
					}
				}
			}
		}
	}
}

func (s *Session) SendStream(ctx context.Context, prompt string) (io.ReadCloser, error) {
	// For PTY backends, fall back to Send
	response, err := s.Send(ctx, prompt)
	if err != nil {
		return nil, err
	}
	return io.NopCloser(strings.NewReader(response)), nil
}

func (s *Session) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.ptmx == nil {
		return fmt.Errorf("session not running")
	}

	// Signal waiters
	select {
	case <-s.stopCh:
	default:
		close(s.stopCh)
	}

	// Send CTRL+C
	_, err := s.ptmx.Write([]byte{0x03})
	return err
}

func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cmd != nil && s.cmd.Process != nil {
		s.cmd.Process.Kill()
		s.cmd.Wait()
	}
	if s.ptmx != nil {
		s.ptmx.Close()
		s.ptmx = nil
	}
	s.pid = 0
	s.state = "exited"
	return nil
}
