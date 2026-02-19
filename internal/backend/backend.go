// Package backend defines the interface for chat backends
package backend

import (
	"context"
	"errors"
	"io"
	"time"
)

var (
	// ErrBackendNotFound is returned when a requested backend is not registered
	ErrBackendNotFound = errors.New("backend not found")
)

// SessionOptions contains options for creating a session
type SessionOptions struct {
	CWD   string
	Role  string
	Tasks []string
}

// Backend represents any chat backend (CLI tool via PTY, or direct API)
type Backend interface {
	// Name returns the backend name
	Name() string

	// CreateSession creates a new session instance
	CreateSession(ctx context.Context, opts SessionOptions) (Session, error)
}

// Session represents an active conversation session
type Session interface {
	// ID returns the unique session identifier
	ID() string

	// State returns current state:
	//   "starting" - Backend process launching, waiting for ready signal
	//   "idle"     - Backend ready to receive prompts
	//   "running"  - Backend processing a prompt
	//   "stopped"  - Backend process terminated, tmux window preserved, can be restarted via Restart()
	//   "error"    - Startup failed or unrecoverable error
	//   "exited"   - Session closed, tmux window destroyed, cannot be restarted
	State() string

	// Refresh retries startup if in error state, or refreshes state detection
	Refresh(ctx context.Context) error

	// Restart stops the backend process (if running) and starts it again
	// using the same command and configuration. Updates PID accordingly.
	Restart(ctx context.Context) error

	// Send sends a prompt and returns the response
	// Blocks until response is complete
	Send(ctx context.Context, prompt string) (string, error)

	// SendStream sends a prompt and streams the response
	// Returns a reader that yields response chunks as they arrive
	SendStream(ctx context.Context, prompt string) (io.ReadCloser, error)

	// Stop interrupts the current operation (best effort)
	Stop(ctx context.Context) error

	// Close terminates the session and cleans up resources
	Close() error

	// Metadata returns session metadata (useful for display/debugging)
	Metadata() SessionMetadata

	// Commands returns command handler (optional, may be nil)
	Commands() CommandHandler

	// SetAlias sets a user-friendly alias for the session
	SetAlias(alias string)
	
	// Role returns the session role (empty if not specified)
	Role() string
	
	// Tasks returns the session tasks (empty if not specified)
	Tasks() []string
	
	// CreatedAt returns when the session was created
	CreatedAt() time.Time
}

// SessionMetadata provides information about a session
type SessionMetadata struct {
	Pid       int               // Process ID (0 if not applicable)
	Cwd       string            // Working directory
	Alias     string            // User-assigned alias
	Backend   string            // Backend name (e.g., "kiro-cli", "claude")
	CreatedAt time.Time         // Session creation time
	WinID     int               // Acme window ID (0 if not applicable)
	Extra     map[string]string // Backend-specific metadata
}

// CommandHandler handles backend-specific commands (optional)
type CommandHandler interface {
	// IsSupported checks if a command is supported
	IsSupported(command string) bool

	// Execute executes a backend-specific command
	// Returns output and error
	Execute(ctx context.Context, command string) (string, error)
}
