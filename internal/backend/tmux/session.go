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
	"syscall"
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
	role        string   // session role (e.g., "developer", "reviewer")
	tasks       []string // session tasks (e.g., ["git", "docker"])
	winID       int      // Acme window ID (0 if not applicable)
	pid         int
	state       string
	context     string // prepended to every prompt
	createdAt   time.Time

	fifoPath string
	fifo     *os.File
	dataCh   chan []byte
	stopCh   chan struct{}
	output   bytes.Buffer

	// Chat log: persistent conversation history (USER/ASSISTANT)
	chatLog     bytes.Buffer
	chatLogSize int64 // tracks size to enforce 2MB limit
	logWaiters  []chan struct{} // notify when new log data arrives

	commands       backend.CommandHandler
	startupHandler StartupHandler
	stateInspector StateInspector

	// For restart support
	backendCommand     []string          // Original backend command (e.g., ["claude", ...])
	environment        map[string]string // Environment variables
	originalCommandStr string            // Full command string as sent to tmux (for restart)

	transitioning   bool // True during Stop/Restart/Close operations to prevent concurrent modifications
	readerGeneration int  // Incremented on each restart to prevent old readers from updating state
	intentionallyStopped bool // True if user explicitly stopped the session (prevents auto-restart)
	lastRestartAttempt time.Time // Last time auto-restart was attempted (prevents spam)
	
	// State machine
	idleCond *sync.Cond  // Signals when state transitions to idle
	idleSince time.Time  // Timestamp when session last transitioned to idle

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

// TryReserve is deprecated and always returns true for idle sessions.
func (s *Session) TryReserve() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state == "idle"
}

// ReleaseReservation is deprecated and does nothing.
func (s *Session) ReleaseReservation() {
}

// TransitionTo attempts to transition to a new state.
// Returns error if transition is invalid.
func (s *Session) TransitionTo(newState string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.transitionToLocked(newState)
}

// IdleDuration returns how long the session has been idle.
// Returns 0 if not currently idle.
func (s *Session) IdleDuration() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state != "idle" || s.idleSince.IsZero() {
		return 0
	}
	return time.Since(s.idleSince)
}

func (s *Session) transitionToLocked(newState string) error {
	// Validate transition
	switch {
	case s.state == "starting" && newState == "idle":
		s.state = newState
		s.idleSince = time.Now()
		s.idleCond.Broadcast()
		return nil
	case s.state == "starting" && newState == "error":
		s.state = newState
		return nil
	case s.state == "idle" && newState == "running":
		s.state = newState
		return nil
	case s.state == "running" && newState == "idle":
		s.state = newState
		s.idleSince = time.Now()
		s.idleCond.Broadcast()
		return nil
	case s.state == "running" && newState == "error":
		s.state = newState
		s.idleCond.Broadcast()
		return nil
	case s.state == "error" && newState == "idle":
		// Manual recovery from error state
		s.state = newState
		s.idleSince = time.Now()
		s.idleCond.Broadcast()
		return nil
	case s.state == "error" && newState == "starting":
		// Restart from error state
		s.state = newState
		return nil
	case newState == "stopped" || newState == "exited":
		// Allow transition to stopped/exited from any state
		s.state = newState
		s.idleCond.Broadcast()
		return nil
	case s.state == "stopped" && newState == "starting":
		// Allow restart from stopped state
		s.state = newState
		return nil
	default:
		return fmt.Errorf("invalid state transition: %s → %s", s.state, newState)
	}
}

// SetState sets the session state (used for explicit signaling)
func (s *Session) SetAlias(alias string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.alias = alias
}

// appendToChatLogLocked appends a message to the chat log with role prefix and separator.
// Enforces 2MB size limit by truncating oldest messages if needed.
// Caller must hold s.mu.
func (s *Session) appendToChatLogLocked(role, content string) {
	const maxLogSize = 2 * 1024 * 1024 // 2MB

	// Format: ROLE:\ncontent\n---\n
	msg := fmt.Sprintf("%s:\n%s\n---\n", role, content)
	msgSize := int64(len(msg))

	// If adding this message would exceed limit, truncate from beginning
	if s.chatLogSize+msgSize > maxLogSize {
		// Calculate how much to truncate
		excessSize := (s.chatLogSize + msgSize) - maxLogSize

		// Get current log content
		logBytes := s.chatLog.Bytes()

		// Find a good truncation point (after a separator to keep messages intact)
		truncateAt := int64(0)
		separatorSize := int64(len("\n---\n"))

		// Search for separator after excess size
		for i := excessSize; i < int64(len(logBytes))-separatorSize; i++ {
			if string(logBytes[i:i+separatorSize]) == "\n---\n" {
				truncateAt = i + separatorSize
				break
			}
		}

		// If no separator found, truncate at excess size (edge case)
		if truncateAt == 0 {
			truncateAt = excessSize
		}

		// Reset buffer with truncated content
		s.chatLog.Reset()
		s.chatLog.Write(logBytes[truncateAt:])
		s.chatLogSize = int64(s.chatLog.Len())
	}

	// Append new message
	s.chatLog.WriteString(msg)
	s.chatLogSize += msgSize

	// Notify all waiting readers
	for _, ch := range s.logWaiters {
		close(ch)
	}
	s.logWaiters = nil
}

// AppendToChatLog appends a message to the chat log with role prefix and separator.
// Enforces 2MB size limit by truncating oldest messages if needed.
func (s *Session) AppendToChatLog(role, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.appendToChatLogLocked(role, content)
}

// GetChatLog returns the full chat log contents
func (s *Session) GetChatLog() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.chatLog.String()
}

// GetChatLogFrom returns chat log data starting from offset.
// Returns (data, hasMore) where hasMore indicates if there's more data available.
func (s *Session) GetChatLogFrom(offset int64) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	logData := s.chatLog.String()
	if offset >= int64(len(logData)) {
		return "", false
	}
	return logData[offset:], true
}

// WaitForLogData returns a channel that will be closed when new log data arrives.
// The caller should check the current log size before waiting.
func (s *Session) WaitForLogData() <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch := make(chan struct{})
	s.logWaiters = append(s.logWaiters, ch)
	return ch
}

func (s *Session) Metadata() backend.SessionMetadata {
	s.mu.Lock()
	defer s.mu.Unlock()

	return backend.SessionMetadata{
		Pid:       s.pid,
		Cwd:       s.cwd,
		Alias:     s.alias,
		Backend:   s.backendName,
		CreatedAt: s.createdAt,
		WinID:     s.winID,
		Extra: map[string]string{
			"tmux_session": s.tmuxSession,
			"tmux_window":  s.windowName,
		},
	}
}

func (s *Session) Commands() backend.CommandHandler {
	return s.commands
}

// Role returns the session role
func (s *Session) Role() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.role
}

// Tasks returns the session tasks
func (s *Session) Tasks() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tasks
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

// SetWinID sets the Acme window ID
func (s *Session) SetWinID(winID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.winID = winID
}

// reader continuously reads from FIFO and sends to dataCh
func (s *Session) reader() {
	// Capture the reader generation at start to detect if we've been superseded
	s.mu.Lock()
	myGeneration := s.readerGeneration
	myDataCh := s.dataCh // Capture our channel reference
	s.mu.Unlock()

	buf := make([]byte, 4096)
	for {
		n, err := s.fifo.Read(buf)
		if err != nil {
			// Only log unexpected errors (not EOF or closed file during shutdown)
			if err != io.EOF && !strings.Contains(err.Error(), "file already closed") {
				debug.Log("[session %s] reader error: %v", s.id, err)
			}

			// Handle EOF/error with single lock acquisition to avoid races
			s.mu.Lock()
			superseded := s.readerGeneration != myGeneration
			exited := s.state == "exited"

			// Only update state if we're still the current reader
			if !superseded && !exited {
				// Only set to stopped if not explicitly closed
				s.transitionToLocked("stopped")
				s.pid = 0
			}
			s.mu.Unlock()

			// Exit immediately if superseded or explicitly closed
			if superseded || exited {
				close(myDataCh)
				debug.Log("[session %s] reader exiting (exited=%v, superseded=%v)",
					s.id, exited, superseded)
				return
			}

			// For stopped state, keep reader alive but idle
			// Wait for either Restart (will increment readerGeneration) or Close (will set state=exited)
			// Poll state every 100ms instead of busy-looping on EOF
			debug.Log("[session %s] reader waiting in stopped state", s.id)
			for {
				time.Sleep(100 * time.Millisecond)
				s.mu.Lock()
				superseded := s.readerGeneration != myGeneration
				exited := s.state == "exited"
				s.mu.Unlock()

				if superseded || exited {
					close(myDataCh)
					debug.Log("[session %s] reader exiting after stopped wait (exited=%v, superseded=%v)",
						s.id, exited, superseded)
					return
				}
				// Still stopped, continue waiting
			}
		}
		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])

			// Check if we've been superseded before sending (single lock check)
			s.mu.Lock()
			superseded := s.readerGeneration != myGeneration
			s.mu.Unlock()

			if superseded {
				close(myDataCh)
				debug.Log("[session %s] reader exiting (superseded during send)", s.id)
				return
			}

			myDataCh <- data
		}
	}
}

func (s *Session) waitForReady(ctx context.Context, timeout time.Duration) error {
	const minContent = 50
	const quiescence = 300 * time.Millisecond
	const pollInterval = 100 * time.Millisecond

	deadline := time.Now().Add(timeout)
	startupDone := s.startupHandler == nil // If no handler, startup is already "done"
	var quiesceStart time.Time

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
			quiesceStart = time.Time{} // Reset quiescence on new data

			// Check for error patterns that indicate launch failure
			// But only if the backend hasn't shown its ready prompt yet
			hasPrompt := strings.Contains(output, "!>")
			if !hasPrompt && (strings.Contains(output, "Failed to apply sandbox") ||
			   strings.Contains(output, "missing kernel Landlock support")) {
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
					quiesceStart = time.Time{} // Reset quiescence after startup
				}
			}

		case <-time.After(pollInterval):
			// Only check for ready after startup is complete
			if !startupDone {
				continue
			}

			// Check if past deadline
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

			// Start or continue quiescence timer
			if quiesceStart.IsZero() {
				quiesceStart = time.Now()
				continue
			}

			// Check if quiescence period has passed
			if time.Since(quiesceStart) >= quiescence {
				debug.Log("[session %s] ready detected (quiescence, %d bytes)", s.id, s.output.Len())
				return nil
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

	if s.state == "stopped" {
		s.mu.Unlock()
		return "", fmt.Errorf("session stopped (use Restart to restart)")
	}

	if s.state != "idle" {
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

	// Save original prompt for chat log (before modification)
	originalPrompt := prompt

	// Instruction to send response to user via mailbox (triggers idle transition)
	outInstruction := fmt.Sprintf("IMPORTANT: When done, write your response summary to user by running:\ncat > /tmp/msg.json <<'MSGEOF'\n{\"to\":\"user\",\"type\":\"STATUS_UPDATE\",\"subject\":\"Response\",\"body\":\"YOUR_SUMMARY_HERE\"}\nMSGEOF\n9p write agent/%s/outbox/msg-$(date +%%s).json < /tmp/msg.json\n\nThe summary MUST include your actual response and key actions taken.\n", s.id)

	// Prepend context if set (skip for slash commands)
	if s.context != "" && !strings.HasPrefix(prompt, "/") {
		prompt = outInstruction + "\n" + s.context + "\n\n" + prompt
	} else if !strings.HasPrefix(prompt, "/") {
		prompt = outInstruction + "\n" + prompt
	}

	// Log user prompt to chat log (already holding lock)
	s.appendToChatLogLocked("USER", originalPrompt)

	// Reset stopCh for this request
	select {
	case <-s.stopCh:
		s.stopCh = make(chan struct{})
	default:
	}

	// Hook will transition to running, just reset output
	s.output.Reset()

	// Release lock during long-running operations so state can be read
	s.mu.Unlock()

	debug.Log("[session %s] sending: %q", s.id, prompt)

	// Send prompt using literal mode to avoid special character interpretation
	if err := sendLiteral(s.target(), prompt); err != nil {
		s.TransitionTo("idle")
		return "", fmt.Errorf("send literal failed: %w", err)
	}

	// Send Enter (C-m) to submit the prompt
	if err := sendKeys(s.target(), "C-m"); err != nil {
		s.TransitionTo("idle")
		return "", fmt.Errorf("send enter failed: %w", err)
	}

	// Hook handles state transitions
	return "", nil
}

func (s *Session) SendStream(ctx context.Context, prompt string) (io.ReadCloser, error) {
	// For tmux backends, fall back to Send
	response, err := s.Send(ctx, prompt)
	if err != nil {
		return nil, err
	}
	return io.NopCloser(strings.NewReader(response)), nil
}

// stopInternal performs the actual stop operation.
// Caller must hold s.mu and have set s.transitioning = true.
// This method will unlock s.mu while performing stop operations.
func (s *Session) stopInternal(ctx context.Context) error {
	// Lock is held by caller
	if s.state == "stopped" || s.state == "exited" {
		return nil // Already stopped
	}

	if s.fifo == nil {
		return fmt.Errorf("session not running")
	}

	// Signal waiters first
	select {
	case <-s.stopCh:
	default:
		close(s.stopCh)
	}

	target := s.target()
	pid := s.pid
	sessionID := s.id
	tmuxSession := s.tmuxSession
	windowName := s.windowName

	s.mu.Unlock()

	// 1. Send Ctrl+C to interrupt any running operation
	// This sends SIGINT to the foreground process group (the backend process)
	if err := sendKeys(target, "C-c"); err != nil {
		debug.Log("[session %s] failed to send Ctrl+C: %v", sessionID, err)
		return fmt.Errorf("failed to interrupt: %w", err)
	}

	// 2. Wait for backend to respond to SIGINT and exit gracefully
	time.Sleep(500 * time.Millisecond)

	// 3. Check if backend process (pid) is still alive and force-kill if necessary
	// Note: pid is the backend process (claude/kiro-cli), NOT the bash shell
	if pid != 0 {
		// Check if process is still alive
		if err := syscall.Kill(pid, 0); err == nil {
			// Process still exists, send another Ctrl+C (some CLIs need 2 Ctrl+C to exit)
			debug.Log("[session %s] backend still alive after first Ctrl+C, sending second", sessionID)
			if err := sendKeys(target, "C-c"); err != nil {
				debug.Log("[session %s] failed to send second Ctrl+C: %v", sessionID, err)
			}
			time.Sleep(500 * time.Millisecond)

			// Check if still alive after second Ctrl+C
			if err := syscall.Kill(pid, 0); err == nil {
				// Still alive, send SIGTERM
				debug.Log("[session %s] backend still alive, sending SIGTERM to PID %d", sessionID, pid)
				if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
					debug.Log("[session %s] SIGTERM failed: %v", sessionID, err)
				}
				time.Sleep(300 * time.Millisecond)

				// Last resort: SIGKILL
				if err := syscall.Kill(pid, 0); err == nil {
					debug.Log("[session %s] backend still alive, sending SIGKILL to PID %d", sessionID, pid)
					if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
						debug.Log("[session %s] SIGKILL failed: %v", sessionID, err)
					}
					time.Sleep(100 * time.Millisecond)
				}
			}
		} else {
			debug.Log("[session %s] backend exited after Ctrl+C", sessionID)
		}
	}

	// 4. Verify tmux window still exists
	if !windowExists(tmuxSession, windowName) {
		debug.Log("[session %s] tmux window was unexpectedly destroyed", sessionID)
		s.mu.Lock()
		s.transitionToLocked("exited")
		s.pid = 0
		return fmt.Errorf("tmux window was unexpectedly destroyed")
	}

	// 5. Update state
	s.mu.Lock()
	s.transitionToLocked("stopped")
	s.pid = 0
	// Note: Reset stopCh for next operation
	s.stopCh = make(chan struct{})

	debug.Log("[session %s] stopped successfully", sessionID)

	// Note: Shell, tmux window, FIFO, and reader goroutine remain active
	// This allows the session to be restarted without recreating the window
	// The reader stays in a wait loop until either Restart() or Close()
	return nil
}

func (s *Session) Stop(ctx context.Context) error {
	s.mu.Lock()

	if s.state == "stopped" || s.state == "exited" {
		s.mu.Unlock()
		return nil // Already stopped
	}

	// Check if another operation is in progress
	if s.transitioning {
		s.mu.Unlock()
		return fmt.Errorf("another operation in progress")
	}
	s.transitioning = true
	s.intentionallyStopped = true // Mark as intentional stop

	// stopInternal returns with lock held, so defer just clears flag and unlocks
	defer func() {
		s.transitioning = false
		s.mu.Unlock()
	}()

	// stopInternal will unlock s.mu and re-lock before returning
	return s.stopInternal(ctx)
}

// Restart stops the backend process (if running) and starts it again
// using the same command and configuration. Updates PID accordingly.
func (s *Session) Restart(ctx context.Context) error {
	s.mu.Lock()

	// Check if we can restart from this state
	if s.state == "exited" {
		s.mu.Unlock()
		return fmt.Errorf("cannot restart exited session (tmux window destroyed)")
	}

	if s.originalCommandStr == "" {
		s.mu.Unlock()
		return fmt.Errorf("original command not available for restart")
	}

	// Check if another operation is in progress
	if s.transitioning {
		s.mu.Unlock()
		return fmt.Errorf("another operation in progress")
	}
	s.transitioning = true
	s.intentionallyStopped = false // Clear intentional stop flag on restart
	s.mu.Unlock()

	// Clear transitioning flag on exit (any error or success path)
	defer func() {
		s.mu.Lock()
		s.transitioning = false
		s.mu.Unlock()
	}()

	// 1. Stop existing process if running (stopInternal expects lock held)
	s.mu.Lock()
	if s.state != "stopped" {
		if err := s.stopInternal(ctx); err != nil {
			s.mu.Unlock()
			return fmt.Errorf("failed to stop: %w", err)
		}
		// stopInternal returns with lock held
		s.mu.Unlock()
	} else {
		s.mu.Unlock()
	}

	s.mu.Lock()
	target := s.target()
	cwd := s.cwd
	cmdStr := s.originalCommandStr
	fifoPath := s.fifoPath
	environment := s.environment
	s.mu.Unlock()

	// 2. Close old FIFO if still open
	s.mu.Lock()
	if s.fifo != nil {
		s.fifo.Close()
		s.fifo = nil
	}
	s.mu.Unlock()

	// 3. Close existing pipe-pane
	if err := closePipePane(target); err != nil {
		debug.Log("[session %s] warning: failed to close pipe-pane: %v", s.id, err)
	}

	// 4. Remove old FIFO file
	os.Remove(fifoPath)

	// Small delay to ensure cleanup
	time.Sleep(100 * time.Millisecond)

	// 5. Recreate FIFO
	if err := syscall.Mkfifo(fifoPath, 0600); err != nil && !os.IsExist(err) {
		return fmt.Errorf("failed to create FIFO: %w", err)
	}

	// 6. Re-setup pipe-pane for output capture
	if err := setupPipePane(target, fifoPath); err != nil {
		return fmt.Errorf("failed to re-setup pipe-pane: %w", err)
	}

	// 7. Open FIFO for reading (this blocks until writer connects)
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

	// 8. Change to working directory (may have changed in shell)
	if err := sendKeys(target, fmt.Sprintf("cd \"%s\"", cwd), "C-m"); err != nil {
		return fmt.Errorf("failed to change directory: %w", err)
	}

	// Small delay for cd to complete
	time.Sleep(100 * time.Millisecond)

	// 9. Restore environment variables
	for k, v := range environment {
		if err := setEnvironment(target, k, v); err != nil {
			return fmt.Errorf("failed to set environment: %w", err)
		}
	}

	// 10. Send the original command to tmux
	if err := sendKeys(target, cmdStr, "C-m"); err != nil {
		return fmt.Errorf("failed to start command: %w", err)
	}

	// 11. Wait for FIFO to open
	var fifo *os.File
	select {
	case fifo = <-fifoOpenCh:
		// Success
	case err := <-fifoErrCh:
		return fmt.Errorf("failed to open FIFO: %w", err)
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timeout opening FIFO")
	}

	// 12. Get new PID (find the backend process, not just the bash shell)
	time.Sleep(500 * time.Millisecond)
	panePID, err := getPanePID(target)
	if err != nil {
		debug.Log("[session %s] warning: failed to get pane PID: %v", s.id, err)
		panePID = 0
	}

	// Find the actual backend process (child of bash shell)
	pid := 0
	if panePID != 0 {
		pid = FindBackendPID(panePID)
		if pid == 0 {
			debug.Log("[session %s] warning: backend process not found for pane PID %d", s.id, panePID)
		} else {
			debug.Log("[session %s] found backend PID %d (pane PID: %d)", s.id, pid, panePID)
		}
	}

	s.mu.Lock()
	s.pid = pid
	s.transitionToLocked("starting")
	s.fifo = fifo
	s.dataCh = make(chan []byte, 100)
	s.stopCh = make(chan struct{})
	s.output.Reset()
	s.readerGeneration++ // Increment to invalidate old reader
	s.mu.Unlock()

	// 13. Start new reader goroutine
	go s.reader()

	// 14. Wait for ready (reuse existing waitForReady logic)
	if err := s.waitForReady(ctx, 30*time.Second); err != nil {
		s.TransitionTo("error")
		return fmt.Errorf("restart failed: %w", err)
	}

	s.TransitionTo("idle")

	debug.Log("[session %s] restarted successfully (pid=%d)", s.id, pid)
	return nil
}

func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if another operation is in progress
	if s.transitioning {
		return fmt.Errorf("another operation in progress")
	}
	s.transitioning = true
	// Note: We don't clear transitioning since Close is final

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

// Refresh re-detects state based on process activity
func (s *Session) Refresh(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == "exited" {
		// Cannot refresh exited sessions
		return nil
	}

	// Check if PID is valid and process is actually running
	if s.pid != 0 {
		// Verify the process exists by sending signal 0
		if err := syscall.Kill(s.pid, 0); err != nil {
			// Process is not running - this is an unexpected crash
			debug.Log("[session %s] refresh: PID %d not running (unexpected crash)", s.id, s.pid)
			
			// Check if this was an intentional stop
			if s.intentionallyStopped {
				debug.Log("[session %s] refresh: intentional stop, not auto-restarting", s.id)
				s.pid = 0
				s.state = "stopped"
				return nil
			}
			
			// Check if we recently attempted a restart (prevent spam)
			if time.Since(s.lastRestartAttempt) < 5*time.Second {
				debug.Log("[session %s] refresh: skipping auto-restart (too soon since last attempt)", s.id)
				s.pid = 0
				s.state = "stopped"
				return nil
			}
			
			// Auto-restart on unexpected crash
			debug.Log("[session %s] refresh: auto-restarting after unexpected crash", s.id)
			s.lastRestartAttempt = time.Now()
			s.mu.Unlock()
			
			err := s.Restart(ctx)
			
			s.mu.Lock()
			if err != nil {
				debug.Log("[session %s] refresh: auto-restart failed: %v", s.id, err)
				s.pid = 0
				s.state = "stopped"
				return nil // Don't propagate error - just mark as stopped
			}
			
			debug.Log("[session %s] refresh: auto-restart successful", s.id)
			return nil
		}
	}

	// If PID is 0, state should be stopped
	if s.pid == 0 {
		debug.Log("[session %s] refresh: PID is 0, setting to stopped", s.id)
		s.state = "stopped"
		return nil
	}

	// If already stopped, don't change state
	if s.state == "stopped" {
		return nil
	}

	// State is managed by explicit signaling, so we don't need to infer it here
	debug.Log("[session %s] refresh: state=%s (managed by explicit signaling)", s.id, s.state)
	return nil
}
