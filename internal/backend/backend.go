// Package backend defines the interface for chat backends
package backend

import (
	"context"
	"io"
	"time"
)

// Backend represents any chat backend (CLI tool via PTY, or direct API)
type Backend interface {
	// Name returns the backend name
	Name() string

	// CreateSession creates a new session instance
	// cwd is the working directory (may be ignored by API backends)
	CreateSession(ctx context.Context, cwd string) (Session, error)
}

// Session represents an active conversation session
type Session interface {
	// ID returns the unique session identifier
	ID() string

	// State returns current state ("starting", "idle", "running", "error: ...", "exited")
	State() string

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
}

// SessionMetadata provides information about a session
type SessionMetadata struct {
	Pid       int               // Process ID (0 if not applicable)
	Cwd       string            // Working directory
	Alias     string            // User-assigned alias
	WinID     int               // Acme window ID
	CreatedAt time.Time         // Session creation time
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
