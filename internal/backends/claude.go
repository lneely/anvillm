// Package backends provides concrete backend implementations
package backends

import (
	"anvillm/internal/backend"
	"anvillm/internal/backend/tmux"
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// NewClaude creates a claude CLI backend
//
// Uses tmux backend to handle interactive TUI dialogs programmatically.
// Runs with --dangerously-skip-permissions because landrun provides the actual sandboxing.
//
// Sessions are automatically saved by Claude to ~/.claude/projects/<dir-path>/<session-id>.jsonl
func NewClaude(nsSuffix string) backend.Backend {
	return newClaudeWithCommand([]string{"claude", "--dangerously-skip-permissions"}, nsSuffix)
}

func newClaudeWithCommand(command []string, nsSuffix string) backend.Backend {
	agentName := os.Getenv("CLAUDE_AGENT_NAME")
	if agentName == "" {
		agentName = "anvillm-agent"
	}
	
	return tmux.New(tmux.Config{
		Name:    "claude",
		Command: append(command, "--agent", agentName),
		Environment: map[string]string{
			"TERM": "xterm-256color",
		},
		TmuxSize: tmux.TmuxSize{
			Rows: 40,
			Cols: 120,
		},
		StartupTime:    15 * time.Second,
		Commands:       &claudeCommands{},
		StartupHandler: &claudeStartupHandler{},
		StateInspector: &claudeStateInspector{},
		NsSuffix:       nsSuffix,
	})
}

// claudeStateInspector implements StateInspector for claude CLI
type claudeStateInspector struct{}

func (i *claudeStateInspector) IsBusy(panePID int) bool {
	// Process tree: pane -> bash -> claude (main process)
	// Claude spawns child processes when executing tools

	// Find bash PID
	bashPID := tmux.FindChildByName(panePID, "bash")
	if bashPID == 0 {
		return false
	}

	// Find claude PID (could be "claude" or "node" running claude)
	claudePID := tmux.FindChildByName(bashPID, "claude")
	if claudePID == 0 {
		// Try "node" as fallback (claude might be running as node process)
		claudePID = tmux.FindChildByName(bashPID, "node")
	}
	if claudePID == 0 {
		return false
	}

	// Check if claude has any children (tool executions, bash spawns, etc.)
	cmd := exec.Command("pgrep", "-P", fmt.Sprintf("%d", claudePID))
	return cmd.Run() == nil
}

// claudeCommands blocks all slash commands with a helpful error
type claudeCommands struct{}

func (h *claudeCommands) IsSupported(command string) bool {
	// Block all slash commands - many open interactive dialogs
	// Users can attach to tmux session if needed: tmux attach -t <session-name>
	return false
}

func (h *claudeCommands) Execute(ctx context.Context, command string) (string, error) {
	return "", fmt.Errorf("slash commands not supported (many open interactive dialogs). To run slash commands, attach to tmux session.")
}

// Session Management Helpers
//
// NOTE: Save/Load operations are NOT supported for Claude backend.
// - Claude auto-saves all conversations to ~/.claude/projects/<dir>/<session-id>.jsonl
// - Sessions are automatically continued via --agent hook integration
// - No practical way to "load" context from one session into another
// - Context sharing would require either:
//   1. Expensive message replay (burns tokens, slow)
//   2. Lossy summarization (defeats purpose of context sharing)
//   3. File manipulation (risky, undefined behavior)

// GetSessionDir returns the directory where Claude stores sessions for the given working directory
func GetSessionDir(cwd string) string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	// Convert cwd to Claude's project path format (replace / with -)
	dirPath := strings.ReplaceAll(cwd, "/", "-")
	return filepath.Join(homeDir, ".claude", "projects", dirPath)
}

// ListSessions returns a list of session IDs for the given directory, sorted by modification time (newest first)
func ListSessions(cwd string) ([]SessionInfo, error) {
	sessionDir := GetSessionDir(cwd)

	files, err := os.ReadDir(sessionDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read session directory: %w", err)
	}

	var sessions []SessionInfo
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".jsonl") {
			sessionID := strings.TrimSuffix(file.Name(), ".jsonl")
			info, _ := file.Info()

			// Try to get summary from file
			summary := getSessionSummary(filepath.Join(sessionDir, file.Name()))

			sessions = append(sessions, SessionInfo{
				ID:       sessionID,
				Summary:  summary,
				ModTime:  info.ModTime(),
			})
		}
	}

	// Sort by modification time (newest first)
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].ModTime.After(sessions[j].ModTime)
	})

	return sessions, nil
}

// SessionInfo contains information about a Claude session
type SessionInfo struct {
	ID      string
	Summary string
	ModTime time.Time
}

// getSessionSummary extracts a summary from the session JSONL file
func getSessionSummary(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return "conversation"
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()

		// Look for summary field
		if strings.Contains(line, `"summary"`) {
			if idx := strings.Index(line, `"summary":"`); idx != -1 {
				start := idx + len(`"summary":"`)
				if end := strings.Index(line[start:], `"`); end != -1 {
					extracted := line[start : start+end]
					if extracted != "" {
						return extracted
					}
				}
			}
		}

		// Fallback: first user message
		if strings.Contains(line, `"role":"user"`) && strings.Contains(line, `"content"`) {
			if idx := strings.Index(line, `"content":"`); idx != -1 {
				start := idx + len(`"content":"`)
				if end := strings.Index(line[start:], `"`); end != -1 {
					content := line[start : start+end]
					if len(content) > 50 {
						content = content[:50] + "..."
					}
					if content != "" {
						return content
					}
				}
			}
		}
	}

	return "conversation"
}

// GetMostRecentSessionID returns the ID of the most recently modified session for the given directory
func GetMostRecentSessionID(cwd string) (string, error) {
	sessions, err := ListSessions(cwd)
	if err != nil {
		return "", err
	}
	if len(sessions) == 0 {
		return "", fmt.Errorf("no sessions found")
	}
	return sessions[0].ID, nil
}

// claudeStartupHandler handles the --dangerously-skip-permissions dialog
type claudeStartupHandler struct {
	sentDown  bool
	sentEnter bool
}

func (h *claudeStartupHandler) HandleStartup(output string) (keys string, done bool) {
	clean := stripANSI(output)

	// Look for the permissions dialog
	if strings.Contains(clean, "WARNING: Claude Code running in Bypass Permissions mode") {
		// Check if we're on "No, exit" (default selection)
		if strings.Contains(clean, "❯ 1. No, exit") && !h.sentDown {
			// Send Down arrow to select "Yes, I accept"
			h.sentDown = true
			return "Down", false
		}
		// Check if we're on "Yes, I accept"
		if strings.Contains(clean, "❯ 2. Yes, I accept") && !h.sentEnter {
			// Send Enter to confirm
			h.sentEnter = true
			return "C-m", false
		}
		// Still showing dialog but not on a specific selection
		// Wait for more output
		return "", false
	}

	// Check if we're past the dialog (looking for greeting)
	greetingPatterns := []string{
		"Hello",
		"Welcome",
		"I'm Claude",
		"What can I help",
		"How can I assist",
	}

	for _, pattern := range greetingPatterns {
		if strings.Contains(clean, pattern) {
			return "", true // Done with startup
		}
	}

	// Not at dialog yet, keep waiting
	return "", false
}
