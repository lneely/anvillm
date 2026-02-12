package tmux

import (
	"anvillm/internal/backend"
	"anvillm/internal/debug"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// Session implements backend.Session for tmux-based tools
type Session struct {
	id          string
	backendName string // backend name (e.g., "kiro-cli", "claude")
	tmuxSession string // persistent tmux session name (e.g., "anvillm-kiro-cli")
	windowName  string // window name within tmux session (same as id)
	cwd         string
	alias       string
	winID       int
	pid         int
	state       string
	lastSummary string
	context     string // prepended to every prompt
	createdAt   time.Time

	fifoPath string
	fifo     *os.File
	dataCh   chan []byte
	stopCh   chan struct{}
	output   bytes.Buffer

	detector       Detector
	cleaner        Cleaner
	commands       backend.CommandHandler
	startupHandler StartupHandler
	stateInspector StateInspector

	mu sync.Mutex
}

// target returns the tmux target for this window (session:window)
func (s *Session) target() string {
	return windowTarget(s.tmuxSession, s.windowName)
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

func (s *Session) LastSummary() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastSummary
}

func (s *Session) Metadata() backend.SessionMetadata {
	s.mu.Lock()
	defer s.mu.Unlock()

	return backend.SessionMetadata{
		Pid:       s.pid,
		Cwd:       s.cwd,
		Alias:     s.alias,
		WinID:     s.winID,
		Backend:   s.backendName,
		CreatedAt: s.createdAt,
		Extra: map[string]string{
			"tmux_session": s.tmuxSession,
			"tmux_window":  s.windowName,
		},
	}
}

func (s *Session) Commands() backend.CommandHandler {
	return s.commands
}

// SetWinID sets the Acme window ID
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

// SetContext sets the context prefix for all prompts
func (s *Session) SetContext(ctx string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.context = ctx
}

// GetContext gets the context prefix
func (s *Session) GetContext() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.context
}

// reader continuously reads from FIFO and sends to dataCh
func (s *Session) reader() {
	buf := make([]byte, 4096)
	for {
		n, err := s.fifo.Read(buf)
		if err != nil {
			// Only log unexpected errors (not EOF or closed file during shutdown)
			if err != io.EOF && !strings.Contains(err.Error(), "file already closed") {
				debug.Log("[session %s] reader error: %v", s.id, err)
			}
			close(s.dataCh)
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
			output := s.output.String()

			// Check for error patterns that indicate launch failure
			if strings.Contains(output, "Error:") ||
			   strings.Contains(output, "ERROR") ||
			   strings.Contains(output, "Failed to apply sandbox") ||
			   strings.Contains(output, "missing kernel Landlock support") {
				fmt.Fprintf(os.Stderr, "Error: backend failed to launch\n")
				fmt.Fprintf(os.Stderr, "Backend output:\n%s\n", output)
				return fmt.Errorf("backend launch failed")
			}

			// If we have a startup handler and it hasn't completed, call it
			if !startupDone && s.startupHandler != nil {
				keys, done := s.startupHandler.HandleStartup(output)
				if keys != "" {
					debug.Log("[session %s] startup handler sending keys: %q", s.id, keys)
					if err := sendKeys(s.target(), keys); err != nil {
						return fmt.Errorf("failed to send startup response: %w", err)
					}
				}
				if done {
					debug.Log("[session %s] startup handler complete", s.id)
					startupDone = true
					s.output.Reset() // Reset output after startup handling
				}
			}

			// Check if ready (only after startup is done)
			if startupDone && s.detector.IsReady(output) {
				debug.Log("[session %s] ready detected", s.id)
				return nil
			}
		case <-time.After(time.Until(deadline)):
			if time.Now().After(deadline) {
				output := s.output.String()
				debug.Log("[session %s] timeout, current output: %s", s.id, output)
				if output != "" {
					fmt.Fprintf(os.Stderr, "Error: timeout waiting for backend to be ready\n")
					fmt.Fprintf(os.Stderr, "Backend output:\n%s\n", output)
				} else {
					fmt.Fprintf(os.Stderr, "Error: timeout waiting for backend to be ready (no output received)\n")
				}
				return fmt.Errorf("timeout waiting for ready")
			}
		}
	}
}

func (s *Session) Send(ctx context.Context, prompt string) (string, error) {
	// Acquire lock for initial validation and state change
	s.mu.Lock()

	if s.fifo == nil {
		s.mu.Unlock()
		return "", fmt.Errorf("session not running")
	}

	if s.state == "starting" {
		s.mu.Unlock()
		return "", fmt.Errorf("session still starting")
	}

	if s.state == "running" {
		s.mu.Unlock()
		return "", fmt.Errorf("session busy")
	}

	// Check command support if applicable
	if strings.HasPrefix(prompt, "/") && s.commands != nil {
		if !s.commands.IsSupported(prompt) {
			cmd := strings.Fields(prompt)[0]
			s.mu.Unlock()
			return "", fmt.Errorf("slash command not supported by %s backend: %s\nTo use manually, middle-click Attach", s.backendName, cmd)
		}
	}

	// Prepend context if set (skip for slash commands)
	if s.context != "" && !strings.HasPrefix(prompt, "/") {
		prompt = s.context + "\n\n" + prompt
	}

	// Reset stopCh for this request
	select {
	case <-s.stopCh:
		s.stopCh = make(chan struct{})
	default:
	}

	s.state = "running"
	s.output.Reset()

	// Release lock during long-running operations so state can be read
	s.mu.Unlock()

	debug.Log("[session %s] sending: %q", s.id, prompt)

	// Send prompt using literal mode to avoid special character interpretation
	if err := sendLiteral(s.target(), prompt); err != nil {
		s.mu.Lock()
		s.state = "idle"
		s.mu.Unlock()
		return "", fmt.Errorf("send literal failed: %w", err)
	}

	// Send Enter
	if err := sendKeys(s.target(), "C-m"); err != nil {
		s.mu.Lock()
		s.state = "idle"
		s.mu.Unlock()
		return "", fmt.Errorf("send enter failed: %w", err)
	}

	// Wait for completion (without holding lock so state can be read)
	if err := s.waitForComplete(ctx, prompt); err != nil {
		s.mu.Lock()
		s.state = "idle"
		s.mu.Unlock()
		return "", err
	}

	// Re-acquire lock for final state change and cleanup
	s.mu.Lock()
	defer s.mu.Unlock()

	s.state = "idle"

	// Clean output
	rawLen := s.output.Len()
	rawOutput := s.output.String()
	result := s.cleaner.Clean(prompt, rawOutput)
	s.lastSummary = summarize(result, 100)
	debug.Log("[session %s] raw: %d bytes, cleaned: %d bytes", s.id, rawLen, len(result))
	if len(result) == 0 && rawLen > 0 {
		debug.Log("[session %s] WARNING: cleaner returned 0 bytes", s.id)
		// Debug: show first and last 500 chars of raw output
		debugFirst := rawOutput
		if len(debugFirst) > 500 {
			debugFirst = debugFirst[:500]
		}
		debugLast := rawOutput
		if len(debugLast) > 500 {
			debugLast = debugLast[len(debugLast)-500:]
		}
		debug.Log("[session %s] DEBUG raw output (first 500 chars): %q", s.id, debugFirst)
		debug.Log("[session %s] DEBUG raw output (last 500 chars): %q", s.id, debugLast)

		// Check for key markers (simple check, no ANSI stripping)
		hasBullet := strings.Contains(rawOutput, "●")
		hasThinking := strings.Contains(rawOutput, "(esc to interrupt)") || strings.Contains(rawOutput, "∴")
		debug.Log("[session %s] DEBUG markers: hasBullet=%v, hasThinking=%v", s.id, hasBullet, hasThinking)

		// Find where the bullet is
		if hasBullet {
			idx := strings.Index(rawOutput, "●")
			start := idx - 50
			if start < 0 {
				start = 0
			}
			end := idx + 100
			if end > len(rawOutput) {
				end = len(rawOutput)
			}
			debug.Log("[session %s] DEBUG bullet context: %q", s.id, rawOutput[start:end])
		}
	}

	return result, nil
}

func (s *Session) waitForComplete(ctx context.Context, prompt string) error {
	const minContent = 50
	const quiescence = 300 * time.Millisecond
	const pollInterval = 100 * time.Millisecond

	var quiesceStart time.Time

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
			quiesceStart = time.Time{} // Reset quiescence on new data

		case <-time.After(pollInterval):
			// Check if we have minimum content
			if s.output.Len() < minContent {
				continue
			}

			// Check process tree: if backend is busy (has child processes), still running
			if s.stateInspector != nil && s.stateInspector.IsBusy(s.pid) {
				quiesceStart = time.Time{} // Reset quiescence
				continue
			}

			// Not busy - start or continue quiescence timer
			if quiesceStart.IsZero() {
				quiesceStart = time.Now()
				continue
			}

			// Check if quiescence period has passed
			if time.Since(quiesceStart) >= quiescence {
				debug.Log("[session %s] idle detected (no tool children, %d bytes)", s.id, s.output.Len())
				return nil
			}
		}
	}
}

func (s *Session) SendAsync(ctx context.Context, prompt string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.fifo == nil {
		return fmt.Errorf("session not running")
	}

	if s.state == "starting" {
		return fmt.Errorf("session still starting")
	}

	if s.state == "running" {
		return fmt.Errorf("session busy")
	}

	// Check command support if applicable
	if strings.HasPrefix(prompt, "/") && s.commands != nil {
		if !s.commands.IsSupported(prompt) {
			cmd := strings.Fields(prompt)[0]
			return fmt.Errorf("slash command not supported by %s backend: %s\nTo use manually, middle-click Attach", s.backendName, cmd)
		}
	}

	// Prepend context if set (skip for slash commands)
	if s.context != "" && !strings.HasPrefix(prompt, "/") {
		prompt = s.context + "\n\n" + prompt
	}

	target := s.target()

	// Send without waiting for completion
	if err := sendLiteral(target, prompt); err != nil {
		return err
	}
	return sendKeys(target, "C-m")
}

func (s *Session) SendStream(ctx context.Context, prompt string) (io.ReadCloser, error) {
	// For tmux backends, fall back to Send
	response, err := s.Send(ctx, prompt)
	if err != nil {
		return nil, err
	}
	return io.NopCloser(strings.NewReader(response)), nil
}

func (s *Session) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.fifo == nil {
		return fmt.Errorf("session not running")
	}

	// Signal waiters
	select {
	case <-s.stopCh:
	default:
		close(s.stopCh)
	}

	// Send CTRL+C
	return sendKeys(s.target(), "C-c")
}

func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	debug.Log("[session %s] Close() called, killing window %s:%s", s.id, s.tmuxSession, s.windowName)

	// 1. Kill tmux window first (closes pipe-pane writer)
	if s.tmuxSession != "" && s.windowName != "" {
		if err := killWindow(s.tmuxSession, s.windowName); err != nil {
			debug.Log("[session %s] killWindow failed: %v", s.id, err)
		} else {
			debug.Log("[session %s] Window %s:%s killed", s.id, s.tmuxSession, s.windowName)
		}
	}

	// 2. Give writer time to close, which will send EOF to reader
	time.Sleep(50 * time.Millisecond)

	// 3. Close FIFO (reader will exit gracefully)
	if s.fifo != nil {
		s.fifo.Close()
		s.fifo = nil
	}

	// 4. Remove FIFO file
	if s.fifoPath != "" {
		os.Remove(s.fifoPath)
		s.fifoPath = ""
	}

	s.pid = 0
	s.state = "exited"
	return nil
}

// summarize returns first n chars of text, trimmed to word boundary
func summarize(text string, n int) string {
	text = strings.TrimSpace(text)
	text = strings.ReplaceAll(text, "\n", " ")
	if len(text) <= n {
		return text
	}
	text = text[:n]
	if i := strings.LastIndex(text, " "); i > n/2 {
		text = text[:i]
	}
	return text + "..."
}
