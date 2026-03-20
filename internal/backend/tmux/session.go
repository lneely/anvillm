package tmux

import (
	"anvillm/internal/backend"
	"anvillm/internal/debug"
	"anvillm/pkg/logging"
	"anvillm/pkg/sandbox"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"syscall"
	"time"

	"go.uber.org/zap"
)

// Session implements backend.Session for tmux-based tools
type Session struct {
	id          string
	backendName string // backend name (e.g., "kiro-cli", "claude")
	tmuxSession string // persistent tmux session name (e.g., "anvillm-0")
	windowName  string // window name within tmux session (same as id)
	cwd         string
	alias       string
	sandbox     string // sandbox config (e.g., "default", "restricted")
	winID       int    // Acme window ID (0 if not applicable)
	pid         int
	state       string
	role               string // bot role (developer, reviewer, tester, etc.)
	context            string // injected into first prompt only
	initialPromptSent  bool   // true after context was sent; reset on clear/compact/stop/restart
	createdAt          time.Time

	stopCh   chan struct{}

	model          string        // Active model override (empty = backend default)
	modelResolver  ModelResolver // Optional: modifies command to include model selection
	clearHandler   ClearHandler  // Optional: backend-specific /clear handling
	resumeHandler  ResumeHandler // Optional: backend-specific resume handling
	compactHandler CompactHandler // Optional: backend-specific /compact handling

	commands       backend.CommandHandler
	stateInspector StateInspector

	// For restart support
	backendCommand     []string          // Original backend command (e.g., ["claude", ...])
	environment        map[string]string // Environment variables
	originalCommandStr string            // Full command string as sent to tmux (for restart)

	transitioning   bool // True during Stop/Restart/Close operations to prevent concurrent modifications
	intentionallyStopped bool // True if user explicitly stopped the session (prevents auto-restart)
	lastRestartAttempt time.Time // Last time auto-restart was attempted (prevents spam)
	hadCrash bool // True if session has crashed at least once (enables resume on restart)
	
	// State machine
	idleCond *sync.Cond  // Signals when state transitions to idle
	idleSince time.Time  // Timestamp when session last transitioned to idle
	
	// Callbacks
	OnStateChange   func(sessionID, oldState, newState string)
	OnCrashRestart  func(sessionID string) // Called after successful crash recovery restart

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
	oldState := s.state
	// Validate transition
	switch {
	case s.state == "starting" && newState == "idle":
		s.state = newState
		s.idleSince = time.Now()
		s.idleCond.Broadcast()
	case s.state == "starting" && newState == "error":
		s.state = newState
	case s.state == "idle" && newState == "running":
		s.state = newState
	case s.state == "running" && newState == "idle":
		s.state = newState
		s.idleSince = time.Now()
		s.idleCond.Broadcast()
	case s.state == "running" && newState == "error":
		s.state = newState
		s.idleCond.Broadcast()
	case s.state == "error" && newState == "idle":
		// Manual recovery from error state
		s.state = newState
		s.idleSince = time.Now()
		s.idleCond.Broadcast()
	case s.state == "error" && newState == "starting":
		// Restart from error state
		debug.Log("[session %s] restarting from error state", s.id)
		s.state = newState
	case newState == "stopped" || newState == "killed":
		// Allow transition to stopped/exited from any state
		debug.Log("[session %s] stopped (old=%s, new=%s)", s.id, oldState, newState)
		s.state = newState
		s.idleCond.Broadcast()
	case s.state == "stopped" && newState == "starting":
		// Allow restart from stopped state
		debug.Log("[session %s] restarting from stopped state", s.id)
		s.state = newState
	default:
		return fmt.Errorf("invalid state transition: %s → %s", s.state, newState)
	}
	
	if s.OnStateChange != nil && oldState != newState {
		go s.OnStateChange(s.id, oldState, newState)
	}
	return nil
}

// SetState sets the session state (used for explicit signaling)
func (s *Session) SetAlias(alias string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.alias = alias
	setWindowOption(s.target(), "ANVILLM_ALIAS", alias)
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

// Sandbox returns the session sandbox config
func (s *Session) Sandbox() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sandbox
}

// Model returns the active model override (empty = backend default)
func (s *Session) Model() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.model
}

// CreatedAt returns when the session was created
func (s *Session) CreatedAt() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.createdAt
}

// GetPid gets the process ID
func (s *Session) GetPid() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pid
}

// SetContext sets the startup context injected into the first prompt only
func (s *Session) SetContext(ctx string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.context = ctx
	s.initialPromptSent = false
}

// GetContext gets the startup context (empty after first Send)
func (s *Session) GetContext() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.context
}

// SetRole sets the bot role
func (s *Session) SetRole(role string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.role = role
	setWindowOption(s.target(), "ANVILLM_ROLE", role)
}

// GetRole gets the bot role
func (s *Session) GetRole() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.role
}

// SetWinID sets the Acme window ID
func (s *Session) SetWinID(winID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.winID = winID
}

func (s *Session) Send(ctx context.Context, prompt string) (string, error) {
	// Acquire lock for initial validation and state change
	s.mu.Lock()

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
			fields := strings.Fields(prompt)
			cmd := "unknown"
			if len(fields) > 0 {
				cmd = fields[0]
			}
			s.mu.Unlock()
			return "", fmt.Errorf("slash command not supported by %s backend: %s\nTo use manually, middle-click Attach", s.backendName, cmd)
		}
	}

	logging.Logger().Debug("sending prompt to session", zap.String("session", s.id), zap.Int("prompt_length", len(prompt)))

	// Prepend context to first prompt only
	if s.context != "" && !s.initialPromptSent && !strings.HasPrefix(prompt, "/") {
		prompt = s.context + "\n\n" + prompt
		s.initialPromptSent = true
	}

	// Reset stopCh for this request
	select {
	case <-s.stopCh:
		s.stopCh = make(chan struct{})
	default:
	}

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

	logging.Logger().Info("prompt sent to session", zap.String("session", s.id))

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

// SendRaw sends literal text followed by Enter to the tmux session.
// Used for slash commands like /clear and /compact.
func (s *Session) SendRaw(text string) error {
	s.mu.Lock()
	target := s.target()
	s.mu.Unlock()
	return sendKeys(target, text, "C-m")
}

// Clear sends the /clear command using backend-specific handling if available.
func (s *Session) Clear() error {
	s.mu.Lock()
	target := s.target()
	handler := s.clearHandler
	s.initialPromptSent = false
	s.mu.Unlock()

	if handler != nil {
		return handler(target)
	}
	// Default: just send /clear + Enter
	return sendKeys(target, "/clear", "C-m")
}

// Resume resumes the most recent conversation using backend-specific handling.
func (s *Session) Resume() error {
	s.mu.Lock()
	target := s.target()
	handler := s.resumeHandler
	s.mu.Unlock()

	if handler != nil {
		return handler(target)
	}
	return fmt.Errorf("resume not implemented for this backend")
}

// Compact sends the /compact command using backend-specific handling.
func (s *Session) Compact() error {
	s.mu.Lock()
	target := s.target()
	handler := s.compactHandler
	s.initialPromptSent = false
	s.mu.Unlock()

	if handler != nil {
		return handler(target)
	}
	return fmt.Errorf("compact not implemented for this backend")
}

// stopInternal performs the actual stop operation.
// Caller must hold s.mu and have set s.transitioning = true.
// This method will unlock s.mu while performing stop operations.
func (s *Session) stopInternal(ctx context.Context) error {
	// Lock is held by caller
	if s.state == "stopped" || s.state == "killed" {
		return nil // Already stopped
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
		s.transitionToLocked("killed")
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

	if s.state == "stopped" || s.state == "killed" {
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
	s.initialPromptSent = false   // Reset so context is re-sent on next prompt

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
	if s.state == "killed" {
		s.mu.Unlock()
		return fmt.Errorf("cannot restart exited session (tmux window destroyed)")
	}

	if len(s.backendCommand) == 0 {
		s.mu.Unlock()
		return fmt.Errorf("backend command not available for restart")
	}

	// Check if another operation is in progress
	if s.transitioning {
		s.mu.Unlock()
		return fmt.Errorf("another operation in progress")
	}
	s.transitioning = true
	s.intentionallyStopped = false // Clear intentional stop flag on restart
	s.initialPromptSent = false    // Reset so context is re-sent on next prompt
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
	environment := s.environment
	backendName := s.backendName
	backendCommand := s.backendCommand
	sbx := s.sandbox
	hadCrash := s.hadCrash
	model := s.model
	modelResolver := s.modelResolver
	s.mu.Unlock()

	// Apply model resolver if a model is specified
	if model != "" && modelResolver != nil {
		backendCommand = modelResolver(backendCommand, model)
	}

	// Add resume flag if this is a crash restart
	if hadCrash {
		switch backendName {
		case "kiro-cli":
			backendCommand = append(backendCommand, "-r")
			debug.Log("[session %s] restart: adding -r flag for kiro-cli resume", s.id)
		case "claude":
			backendCommand = append(backendCommand, "-c")
			debug.Log("[session %s] restart: adding -c flag for claude continue", s.id)
		}
	}

	// Reload sandbox config from YAML (picks up any changes)
	baseCfg, err := sandbox.Load()
	if err != nil {
		return fmt.Errorf("failed to load global config: %w", err)
	}
	baseLayer := sandbox.LayeredConfig{
		Filesystem: baseCfg.Filesystem,
		Network:    baseCfg.Network,
		Env:        baseCfg.Env,
	}
	layers := []sandbox.LayeredConfig{baseLayer}

	backendLayer, err := sandbox.LoadBackend(backendName)
	if err != nil {
		return fmt.Errorf("failed to load backend config %q: %w", backendName, err)
	}
	layers = append(layers, backendLayer)

	if sbx == "" {
		sbx = sandbox.DefaultSandbox
	}
	sbxLayer, err := sandbox.LoadSandbox(sbx)
	if err != nil {
		return fmt.Errorf("failed to load sandbox %q: %w", sbx, err)
	}
	layers = append(layers, sbxLayer)

	general := sandbox.GeneralConfig{BestEffort: false, LogLevel: "error"}
	advanced := sandbox.AdvancedConfig{LDD: false, AddExec: true}
	sandboxCfg := sandbox.Merge(general, advanced, layers...)

	// Wrap command with updated sandbox config
	command := sandbox.WrapCommand(sandboxCfg, backendCommand, cwd)
	cmdStr := ""
	for i, arg := range command {
		if i > 0 {
			cmdStr += " "
		}
		if containsSpace(arg) {
			cmdStr += fmt.Sprintf("\"%s\"", arg)
		} else {
			cmdStr += arg
		}
	}
	debug.Log("[session %s] restart with updated sandbox: %v", s.id, command)

	// 2. Change to working directory (may have changed in shell)
	if err := sendKeys(target, fmt.Sprintf("cd \"%s\"", cwd), "C-m"); err != nil {
		return fmt.Errorf("failed to change directory: %w", err)
	}

	// Small delay for cd to complete
	time.Sleep(100 * time.Millisecond)

	// 3. Restore environment variables
	for k, v := range environment {
		if err := setEnvironment(target, k, v); err != nil {
			return fmt.Errorf("failed to set environment: %w", err)
		}
		if err := sendKeys(target, fmt.Sprintf("export %s=%s", k, v), "C-m"); err != nil {
			return fmt.Errorf("failed to export environment: %w", err)
		}
	}

	// 4. Send the command to tmux
	if err := sendKeys(target, cmdStr, "C-m"); err != nil {
		return fmt.Errorf("failed to start command: %w", err)
	}

	// 5. Get new PID (find the backend process, not just the bash shell)
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
	s.transitionToLocked("idle")
	s.stopCh = make(chan struct{})
	s.mu.Unlock()

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

	// Kill tmux window
	if s.tmuxSession != "" && s.windowName != "" {
		if err := killWindow(s.tmuxSession, s.windowName); err != nil {
			debug.Log("[session %s] killWindow failed: %v", s.id, err)
		} else {
			debug.Log("[session %s] Window %s:%s killed", s.id, s.tmuxSession, s.windowName)
		}
	}

	s.pid = 0
	s.transitionToLocked("killed")
	return nil
}

// Refresh re-detects state based on process activity
func (s *Session) Refresh(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == "killed" {
		// Cannot refresh exited sessions
		return nil
	}

	// Check if PID is valid and process is actually running
	if s.pid != 0 {
		// Verify the process exists by sending signal 0
		if err := syscall.Kill(s.pid, 0); err != nil {
			// Process is not running - this is an unexpected crash
			debug.Log("[session %s] refresh: PID %d not running (unexpected crash)", s.id, s.pid)
			
			// Mark that we had a crash (enables resume on restart)
			s.hadCrash = true
			
			// Check if this was an intentional stop
			if s.intentionallyStopped {
				debug.Log("[session %s] refresh: intentional stop, not auto-restarting", s.id)
				s.pid = 0
				s.transitionToLocked("stopped")
				return nil
			}
			
			// Check if we recently attempted a restart (prevent spam)
			if time.Since(s.lastRestartAttempt) < 5*time.Second {
				debug.Log("[session %s] refresh: skipping auto-restart (too soon since last attempt)", s.id)
				s.pid = 0
				s.transitionToLocked("stopped")
				return nil
			}
			
			// Auto-restart on unexpected crash
			debug.Log("[session %s] refresh: auto-restarting after unexpected crash", s.id)
			s.lastRestartAttempt = time.Now()
			s.initialPromptSent = false
			onCrashRestart := s.OnCrashRestart
			s.mu.Unlock()
			
			err := s.Restart(ctx)
			
			s.mu.Lock()
			if err != nil {
				debug.Log("[session %s] refresh: auto-restart failed: %v", s.id, err)
				s.pid = 0
				s.transitionToLocked("stopped")
				return nil // Don't propagate error - just mark as stopped
			}
			
			debug.Log("[session %s] refresh: auto-restart successful", s.id)
			if onCrashRestart != nil {
				s.mu.Unlock()
				onCrashRestart(s.id)
				s.mu.Lock()
			}
			return nil
		}
	}

	// If PID is 0, state should be stopped
	if s.pid == 0 {
		debug.Log("[session %s] refresh: PID is 0, setting to stopped", s.id)
		s.transitionToLocked("stopped")
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
