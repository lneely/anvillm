// Package session manages persistent kiro-cli PTY sessions.
package session

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/creack/pty"
)

var sessionCounter uint64

// Session represents a persistent kiro-cli chat session.
type Session struct {
	ID    string
	Alias string
	Cwd   string
	WinID int
	Pid   int
	State string // "starting", "idle", "running", or "error: ..."

	cmd  *exec.Cmd
	ptmx *os.File
	mu   sync.Mutex

	// Channel to receive data from the reader goroutine
	dataCh chan []byte
	// Channel to signal stop
	stopCh chan struct{}
	// Accumulated output
	output bytes.Buffer
}

// Manager holds all active sessions.
type Manager struct {
	sessions map[string]*Session
	mu       sync.RWMutex
}

// NewManager creates a session manager.
func NewManager() *Manager {
	return &Manager{sessions: make(map[string]*Session)}
}

// New creates a new kiro-cli session in the given working directory.
func (m *Manager) New(cwd string) (*Session, error) {
	id := fmt.Sprintf("%d", atomic.AddUint64(&sessionCounter, 1))

	cmd := exec.Command("kiro-cli", "chat", "--trust-all-tools")
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 40, Cols: 120})
	if err != nil {
		return nil, fmt.Errorf("failed to start pty: %w", err)
	}

	s := &Session{
		ID:     id,
		Cwd:    cwd,
		Pid:    cmd.Process.Pid,
		State:  "starting",
		cmd:    cmd,
		ptmx:   ptmx,
		dataCh: make(chan []byte, 100),
		stopCh: make(chan struct{}),
	}

	// Start single reader goroutine
	go s.reader()

	m.mu.Lock()
	m.sessions[id] = s
	m.mu.Unlock()

	// Wait for initial prompt - look for "> " which appears at end of prompt
	err = s.waitForPattern("> ", 30*time.Second)
	if err != nil {
		log.Printf("[session %s] warning: %v", s.ID, err)
	}
	s.State = "idle"
	s.output.Reset()
	log.Printf("[session %s] ready", s.ID)

	return s, nil
}

// reader continuously reads from PTY and sends to dataCh
func (s *Session) reader() {
	buf := make([]byte, 4096)
	for {
		n, err := s.ptmx.Read(buf)
		if err != nil {
			if err != io.EOF {
				log.Printf("[session %s] reader error: %v", s.ID, err)
			}
			close(s.dataCh)
			// Reap the process to avoid zombies
			if s.cmd != nil && s.cmd.Process != nil {
				s.cmd.Wait()
			}
			s.State = "exited"
			s.Pid = 0
			return
		}
		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])
			s.dataCh <- data
		}
	}
}

// waitForPattern waits until the output contains the pattern
func (s *Session) waitForPattern(pattern string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ansiRegex := regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]`)
	for {
		select {
		case data, ok := <-s.dataCh:
			if !ok {
				return fmt.Errorf("session closed")
			}
			s.output.Write(data)
			// Just strip ANSI, don't filter
			clean := ansiRegex.ReplaceAllString(s.output.String(), "")
			if strings.Contains(clean, pattern) {
				return nil
			}
		case <-time.After(time.Until(deadline)):
			return fmt.Errorf("timeout waiting for %q", pattern)
		}
	}
}

// waitForSecondPrompt waits for the response to complete (normal prompts end with "Time:")
func (s *Session) waitForSecondPrompt() error {
	for {
		select {
		case <-s.stopCh:
			return fmt.Errorf("interrupted")
		case data, ok := <-s.dataCh:
			if !ok {
				return fmt.Errorf("session closed")
			}
			s.output.Write(data)
			log.Printf("[session %s] +%d bytes, total %d", s.ID, len(data), s.output.Len())
			
			// Kiro prints "Time:" at the end of every response
			if strings.Contains(s.output.String(), "Time:") {
				// Drain any remaining data in channel
				for {
					select {
					case extra, ok := <-s.dataCh:
						if ok {
							s.output.Write(extra)
						}
					default:
						log.Printf("[session %s] response complete, %d bytes", s.ID, s.output.Len())
						return nil
					}
				}
			}
		}
	}
}

// waitForCommandComplete waits for slash command output (ends with next prompt, not "Time:")
func (s *Session) waitForCommandComplete() error {
	ansiRegex := regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]`)
	var contentLen int
	
	for {
		select {
		case <-s.stopCh:
			return fmt.Errorf("interrupted")
		case data, ok := <-s.dataCh:
			if !ok {
				return fmt.Errorf("session closed")
			}
			s.output.Write(data)
			out := s.output.String()
			log.Printf("[session %s] +%d bytes, total %d", s.ID, len(data), len(out))
			
			// Strip ANSI and \r for checking
			clean := ansiRegex.ReplaceAllString(out, "")
			clean = strings.ReplaceAll(clean, "\r", "")
			
			// Wait for substantial content before looking for end prompt
			if len(clean) > contentLen+50 {
				contentLen = len(clean)
			}
			
			// Look for a new prompt line after we have content
			// Prompt pattern: newline followed by [something] then > 
			if contentLen > 200 && strings.Contains(clean, "\n[") {
				// Check if there's a prompt after content
				lines := strings.Split(clean, "\n")
				for i := len(lines) - 1; i >= 0; i-- {
					if strings.Contains(lines[i], "] ") && strings.Contains(lines[i], ">") {
						// Found end prompt, drain and return
						for {
							select {
							case extra, ok := <-s.dataCh:
								if ok {
									s.output.Write(extra)
								}
							default:
								log.Printf("[session %s] command complete, %d bytes", s.ID, s.output.Len())
								return nil
							}
						}
					}
				}
			}
		}
	}
}

func (s *Session) cleanedOutput() string {
	out := s.output.String()
	// Strip ANSI
	clean := regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]`).ReplaceAllString(out, "")
	clean = strings.ReplaceAll(clean, "\r", "\n")

	lines := strings.Split(clean, "\n")
	var result []string
	inResponse := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check for response marker FIRST - it may be on same line as spinner
		if idx := strings.Index(line, "> "); idx >= 0 {
			// Make sure it's the response marker, not part of prompt (prompts have [agent])
			if !strings.Contains(line, "[") || !strings.Contains(line, "]") {
				inResponse = true
				text := strings.TrimSpace(line[idx+2:])
				if text != "" {
					result = append(result, text)
				}
				continue
			}
		}

		// Skip noise
		if strings.Contains(line, "Thinking...") ||
			strings.Contains(line, "▸ Time:") ||
			(strings.Contains(line, "[") && strings.Contains(line, "]") && strings.Contains(line, ">")) ||
			strings.HasPrefix(trimmed, "⠋") ||
			strings.HasPrefix(trimmed, "⠙") ||
			strings.HasPrefix(trimmed, "⠹") ||
			strings.HasPrefix(trimmed, "⠸") ||
			strings.HasPrefix(trimmed, "⠼") ||
			strings.HasPrefix(trimmed, "⠴") ||
			strings.HasPrefix(trimmed, "⠦") ||
			strings.HasPrefix(trimmed, "⠧") ||
			strings.HasPrefix(trimmed, "⠇") ||
			strings.HasPrefix(trimmed, "⠏") ||
			strings.HasPrefix(trimmed, "⢀") ||
			strings.HasPrefix(trimmed, "⡀") ||
			strings.HasPrefix(trimmed, "✓") {
			continue
		}

		// Keep hooks lines
		if strings.Contains(line, "hooks finished") {
			result = append(result, trimmed)
			continue
		}

		if inResponse && trimmed != "" {
			result = append(result, line)
		}
	}

	return strings.Join(result, "\n")
}

// cleanedCommandOutput extracts output from slash commands (no "> " marker)
func (s *Session) cleanedCommandOutput() string {
	out := s.output.String()
	// Strip ANSI
	clean := regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]`).ReplaceAllString(out, "")
	clean = strings.ReplaceAll(clean, "\r", "\n")

	lines := strings.Split(clean, "\n")
	var result []string
	collecting := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == "" {
			continue
		}

		// Detect prompt lines: [agent] >
		isPrompt := strings.Contains(line, "[") && strings.Contains(line, "]") && strings.Contains(line, ">")

		if isPrompt {
			if collecting {
				// Second prompt = end
				break
			}
			// Skip prompt lines but don't start collecting yet
			continue
		}

		// Skip command echo and noise
		if strings.HasPrefix(trimmed, "/") ||
			strings.Contains(trimmed, "Switching to") ||
			strings.HasPrefix(trimmed, "⠋") ||
			strings.HasPrefix(trimmed, "⠙") ||
			strings.HasPrefix(trimmed, "⠹") ||
			strings.HasPrefix(trimmed, "⠸") ||
			strings.HasPrefix(trimmed, "⠼") ||
			strings.HasPrefix(trimmed, "⠴") ||
			strings.HasPrefix(trimmed, "⠦") ||
			strings.HasPrefix(trimmed, "⠧") ||
			strings.HasPrefix(trimmed, "⠇") ||
			strings.HasPrefix(trimmed, "⠏") ||
			strings.HasPrefix(trimmed, "⢀") ||
			strings.HasPrefix(trimmed, "⡀") {
			continue
		}

		// Collect any non-noise content
		collecting = true
		result = append(result, trimmed)
	}

	return strings.Join(result, "\n")
}

// Get returns a session by ID.
func (m *Manager) Get(id string) *Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[id]
}

// List returns all session IDs.
func (m *Manager) List() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	return ids
}

// Remove removes a session from the manager.
func (m *Manager) Remove(id string) {
	m.mu.Lock()
	delete(m.sessions, id)
	m.mu.Unlock()
}

// supportedCommands lists non-interactive slash commands that work via pty.
var supportedCommands = []string{"/chat load", "/chat save", "/help", "/agent list"}

// Send writes a prompt to the session and waits for response.
func (s *Session) Send(prompt string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.ptmx == nil {
		return "", fmt.Errorf("session not running")
	}

	if s.State == "starting" {
		return "", fmt.Errorf("session still starting, please wait")
	}

	// Reset stopCh for this request
	select {
	case <-s.stopCh:
		// was closed, need new one
		s.stopCh = make(chan struct{})
	default:
	}

	// Check if slash command is supported
	if strings.HasPrefix(prompt, "/") {
		supported := false
		for _, cmd := range supportedCommands {
			if strings.HasPrefix(prompt, cmd) {
				supported = true
				break
			}
		}
		if !supported {
			log.Printf("[session %s] unsupported command %q, supported: %v", s.ID, prompt, supportedCommands)
			return "", fmt.Errorf("unsupported command, supported: %v", supportedCommands)
		}
	}

	s.State = "running"
	s.output.Reset()

	isCommand := strings.HasPrefix(prompt, "/")

	log.Printf("[session %s] sending: %q", s.ID, prompt)
	if _, err := s.ptmx.Write([]byte(prompt + "\r")); err != nil {
		s.State = "idle"
		return "", fmt.Errorf("write failed: %w", err)
	}

	// Slash commands wait for next prompt, normal prompts wait for "Time:"
	var err error
	if isCommand {
		err = s.waitForCommandComplete()
	} else {
		err = s.waitForSecondPrompt()
	}
	if err != nil {
		s.State = "idle"
		return "", err
	}

	s.State = "idle"
	
	var result string
	if isCommand {
		result = s.cleanedCommandOutput()
	} else {
		result = s.cleanedOutput()
	}
	log.Printf("[session %s] response: %d bytes", s.ID, len(result))
	return result, nil
}

// extractResponse extracts the actual response from the output
func (s *Session) extractResponse() string {
	clean := s.cleanedOutput()
	
	// Find content between first "] > ...\n" and last "] > "
	lines := strings.Split(clean, "\n")
	var result []string
	inResponse := false
	
	for _, line := range lines {
		if strings.Contains(line, "] > ") {
			if inResponse {
				break // Second prompt, stop
			}
			inResponse = true
			continue
		}
		if inResponse {
			// Skip spinner and hook lines
			if strings.Contains(line, "Thinking...") ||
				strings.Contains(line, "hooks finished") ||
				strings.HasPrefix(strings.TrimSpace(line), "▸") {
				continue
			}
			if trimmed := strings.TrimSpace(line); trimmed != "" {
				result = append(result, line)
			}
		}
	}
	return strings.Join(result, "\n")
}

// SendCtl sends a control command (like /chat save, /chat load).
func (s *Session) SendCtl(cmd string) error {
	_, err := s.Send(cmd)
	return err
}

// Stop sends SIGINT to interrupt the current operation.
func (s *Session) Stop() error {
	if s.ptmx == nil {
		return fmt.Errorf("session not running")
	}
	// Signal waiters to abort - close channel, will be recreated by Send()
	select {
	case <-s.stopCh:
		// already closed
	default:
		close(s.stopCh)
	}
	// Write ETX (Ctrl+C) to PTY
	_, err := s.ptmx.Write([]byte{0x03})
	return err
}

// Kill terminates the session.
func (s *Session) Kill() error {
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
	s.Pid = 0
	s.State = "idle"
	return nil
}

// Output returns the output from the last command.
func (s *Session) Output() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cleanedOutput()
}

// Err returns accumulated error/debug output.
func (s *Session) Err() string {
	return ""
}

// SessionsDir returns the path to the sessions storage directory.
func SessionsDir() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".config", "acme-q", "sessions")
	os.MkdirAll(dir, 0755)
	return dir
}

// SavePath returns the save file path for a session.
func (s *Session) SavePath() string {
	return filepath.Join(SessionsDir(), s.ID+".json")
}

// GenerateSmartFilename creates a descriptive filename from saved conversation JSON.
// It extracts first few meaningful words from user prompts.
// windowBody is optional extra text (from chat window) to search for content.
func GenerateSmartFilename(jsonPath string, windowBody string) (string, error) {
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return "", err
	}

	// Extract user prompt text - try JSON first, then window body
	var summaryText string

	// Try to find user prompts in the JSON
	promptRe := regexp.MustCompile(`"prompt"\s*:\s*"([^"]{10,200})"`)
	if matches := promptRe.FindAllStringSubmatch(string(data), 5); len(matches) > 0 {
		for _, m := range matches {
			text := m[1]
			// Skip if it looks like JSON/tool output
			if !strings.Contains(text, "exit_status") && !strings.Contains(text, "\\n\\t") {
				summaryText = text
				break
			}
		}
	}

	// If no prompt in JSON, try to extract from window body (first USER: with content)
	if summaryText == "" && windowBody != "" {
		if idx := strings.Index(windowBody, "USER:"); idx >= 0 {
			userText := strings.TrimSpace(windowBody[idx+5:])
			// Take up to next section marker
			if end := strings.Index(userText, "\n\nA:"); end > 0 {
				userText = userText[:end]
			} else if end := strings.Index(userText, "\n\nASSISTANT:"); end > 0 {
				userText = userText[:end]
			} else if end := strings.Index(userText, "\n---"); end > 0 {
				userText = userText[:end]
			}
			userText = strings.TrimSpace(userText)
			if len(userText) > 200 {
				userText = userText[:200]
			}
			if len(userText) > 5 {
				summaryText = userText
			}
		}
	}

	// Extract words for filename
	var parts []string
	if summaryText != "" {
		summaryText = strings.ToLower(summaryText)
		wordRe := regexp.MustCompile(`[a-z]+`)
		words := wordRe.FindAllString(summaryText, 10)

		stopWords := []string{"a", "an", "and", "are", "as", "at", "be", "by", "can", "do", "for", "from", "has", "have", "he", "her", "his", "how", "if", "in", "into", "is", "it", "its", "just", "me", "my", "no", "not", "of", "on", "or", "our", "out", "so", "some", "than", "that", "the", "their", "them", "then", "there", "these", "they", "this", "to", "up", "us", "was", "we", "what", "when", "which", "who", "will", "with", "would", "you", "your"}
		for _, w := range words {
			if len(w) > 2 && !slices.Contains(stopWords, w) {
				parts = append(parts, w)
				if len(parts) >= 5 {
					break
				}
			}
		}
	}

	if len(parts) == 0 {
		parts = append(parts, "chat")
	}

	filename := time.Now().Format("20060102T150405") + "-" + strings.Join(parts, "-") + ".json"
	return filepath.Join(SessionsDir(), filename), nil
}

// Save persists the session conversation.
func (s *Session) Save() error {
	return s.SendCtl("/chat save " + s.SavePath())
}

// Load restores a session conversation from file.
func (s *Session) Load(path string) error {
	return s.SendCtl("/chat load " + path)
}
