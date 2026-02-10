package tmux

import (
	"acme-q/internal/backend"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

// Session implements backend.Session for tmux-based tools
type Session struct {
	id          string
	sessionName string // tmux session name
	cwd         string
	alias       string
	winID       int
	pid         int
	state       string
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
		Extra: map[string]string{
			"tmux_session": s.sessionName,
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

// reader continuously reads from FIFO and sends to dataCh
func (s *Session) reader() {
	buf := make([]byte, 4096)
	for {
		n, err := s.fifo.Read(buf)
		if err != nil {
			// Only log unexpected errors (not EOF or closed file during shutdown)
			if err != io.EOF && !strings.Contains(err.Error(), "file already closed") {
				log.Printf("[session %s] reader error: %v", s.id, err)
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

			log.Printf("[session %s] startup: %d bytes accumulated", s.id, s.output.Len())

			// If we have a startup handler and it hasn't completed, call it
			if !startupDone && s.startupHandler != nil {
				keys, done := s.startupHandler.HandleStartup(s.output.String())
				if keys != "" {
					log.Printf("[session %s] startup handler sending keys: %q", s.id, keys)
					if err := sendKeys(s.sessionName, keys); err != nil {
						return fmt.Errorf("failed to send startup response: %w", err)
					}
				}
				if done {
					log.Printf("[session %s] startup handler complete", s.id)
					startupDone = true
					s.output.Reset() // Reset output after startup handling
				}
			}

			// Check if ready (only after startup is done)
			if startupDone && s.detector.IsReady(s.output.String()) {
				log.Printf("[session %s] ready detected", s.id)
				return nil
			}
		case <-time.After(time.Until(deadline)):
			if time.Now().After(deadline) {
				log.Printf("[session %s] timeout, current output: %s", s.id, s.output.String())
				return fmt.Errorf("timeout waiting for ready")
			}
		}
	}
}

func (s *Session) Send(ctx context.Context, prompt string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.fifo == nil {
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

	log.Printf("[session %s] sending: %q", s.id, prompt)

	// Send prompt using literal mode to avoid special character interpretation
	if err := sendLiteral(s.sessionName, prompt); err != nil {
		s.state = "idle"
		return "", fmt.Errorf("send literal failed: %w", err)
	}

	// Send Enter
	if err := sendKeys(s.sessionName, "C-m"); err != nil {
		s.state = "idle"
		return "", fmt.Errorf("send enter failed: %w", err)
	}

	// Wait for completion
	if err := s.waitForComplete(ctx, prompt); err != nil {
		s.state = "idle"
		return "", err
	}

	s.state = "idle"

	// Clean output
	rawLen := s.output.Len()
	result := s.cleaner.Clean(prompt, s.output.String())
	log.Printf("[session %s] raw: %d bytes, cleaned: %d bytes", s.id, rawLen, len(result))
	if len(result) == 0 && rawLen > 0 {
		log.Printf("[session %s] WARNING: cleaner returned 0 bytes", s.id)
	}

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
			log.Printf("[session %s] +%d bytes, total %d", s.id, len(data), s.output.Len())

			if s.detector.IsComplete(prompt, s.output.String()) {
				// Drain any remaining data
				for {
					select {
					case extra, ok := <-s.dataCh:
						if ok {
							s.output.Write(extra)
						}
					case <-time.After(10 * time.Millisecond):
						log.Printf("[session %s] response complete, %d bytes", s.id, s.output.Len())
						return nil
					}
				}
			}
		}
	}
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
	return sendKeys(s.sessionName, "C-c")
}

func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. Kill tmux session first (closes pipe-pane writer)
	if s.sessionName != "" {
		killSession(s.sessionName)
		s.sessionName = ""
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
