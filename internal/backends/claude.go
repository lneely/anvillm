// Package backends provides concrete backend implementations
package backends

import (
	"anvillm/internal/backend"
	"anvillm/internal/backend/tmux"
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// NewClaude creates a claude CLI backend
//
// Uses tmux backend to handle interactive TUI dialogs programmatically.
// The startup handler automatically accepts the --dangerously-skip-permissions dialog.
//
// Sessions are automatically saved by Claude to ~/.claude/projects/<dir-path>/<session-id>.jsonl
func NewClaude() backend.Backend {
	return newClaudeWithCommand([]string{"claude", "--dangerously-skip-permissions"})
}

// NewClaudeWithContinue creates a backend that continues the most recent conversation
func NewClaudeWithContinue() backend.Backend {
	return newClaudeWithCommand([]string{"claude", "--dangerously-skip-permissions", "--continue"})
}

// NewClaudeWithResume creates a backend that resumes a specific session by ID
func NewClaudeWithResume(sessionID string) backend.Backend {
	return newClaudeWithCommand([]string{"claude", "--dangerously-skip-permissions", "--resume", sessionID})
}

func newClaudeWithCommand(command []string) backend.Backend {
	return tmux.New(tmux.Config{
		Name:    "claude",
		Command: command,
		Environment: map[string]string{
			"TERM":     "xterm-256color",
			"NO_COLOR": "1",
		},
		TmuxSize: tmux.TmuxSize{
			Rows: 40,
			Cols: 120,
		},
		StartupTime:    15 * time.Second,
		Detector:       &claudeDetector{},
		Cleaner:        &claudeCleaner{},
		Commands:       &claudeCommands{},
		StartupHandler: &claudeStartupHandler{},
	})
}

// claudeDetector implements pattern detection for claude CLI
type claudeDetector struct{}

func (d *claudeDetector) IsReady(output string) bool {
	clean := stripANSI(output)

	// Claude shows a greeting when ready
	greetingPatterns := []string{
		"Hello",
		"Welcome",
		"I'm Claude",
		"What can I help",
		"How can I assist",
	}

	for _, pattern := range greetingPatterns {
		if strings.Contains(clean, pattern) {
			return true
		}
	}

	// If we have substantial output, likely ready
	if len(clean) > 50 {
		return true
	}

	return false
}

func (d *claudeDetector) IsComplete(prompt string, output string) bool {
	clean := stripANSI(output)
	clean = strings.ReplaceAll(clean, "\r", "")

	isCommand := strings.HasPrefix(strings.TrimSpace(prompt), "/")

	if isCommand {
		// For slash commands, completion is simpler:
		// Just wait for the prompt to return (bottom chrome appears)
		if len(clean) < 500 {
			return false
		}

		// Get the tail to check for bottom chrome
		tailStart := len(clean) - 500
		if tailStart < 0 {
			tailStart = 0
		}
		tail := clean[tailStart:]

		// Command complete when bottom chrome appears
		hasChromeInTail := strings.Contains(tail, "bypass permissions on") ||
			strings.Contains(tail, "Thinking on (tab to toggle)")

		return hasChromeInTail
	}

	// For normal prompts (non-slash commands):
	// Need substantial output first
	if len(clean) < 1000 {
		return false
	}

	// Generic thinking indicator patterns:
	// - "(esc to interrupt)" appears during processing
	// - "∴" is the thought indicator
	hasSeenThinking := strings.Contains(clean, "(esc to interrupt)") ||
		strings.Contains(clean, "∴")

	if !hasSeenThinking {
		// Not even started processing yet
		return false
	}

	// CRITICAL: Must have seen actual response content (● marker)
	// Claude responses and tool usage both use ●
	hasResponse := strings.Contains(clean, "●")
	if !hasResponse {
		// No response content yet, keep waiting
		return false
	}

	// Get the tail of the output (last 500 chars)
	tailStart := len(clean) - 500
	if tailStart < 0 {
		tailStart = 0
	}
	tail := clean[tailStart:]

	// Check if thinking indicators have stopped in the recent output
	noRecentThinking := !strings.Contains(tail, "(esc to interrupt)") &&
		!strings.Contains(tail, "∴")

	// Check if we have bottom chrome in the tail
	hasChromeInTail := strings.Contains(tail, "bypass permissions on") ||
		strings.Contains(tail, "Thinking on (tab to toggle)")

	// Complete when:
	// 1. We saw thinking indicators earlier (Claude started)
	// 2. We saw response content (● appeared)
	// 3. No thinking indicators in recent output (Claude finished)
	// 4. Chrome is present in recent output (UI returned to ready state)
	// 5. Substantial output accumulated
	return noRecentThinking && hasChromeInTail && len(clean) > 2000
}

// claudeCleaner implements output cleaning for claude CLI
type claudeCleaner struct{}

func (c *claudeCleaner) Clean(prompt string, rawOutput string) string {
	// Strip ANSI codes first
	clean := stripANSI(rawOutput)
	clean = strings.ReplaceAll(clean, "\r", "\n")

	// Remove terminal title sequences (OSC sequences like ]0;...)
	// These appear as ]0;✳ title in the output
	titleRegex := regexp.MustCompile(`\]0;[^\n]*`)
	clean = titleRegex.ReplaceAllString(clean, "")

	isCommand := strings.HasPrefix(strings.TrimSpace(prompt), "/")

	if isCommand {
		// For slash commands, extract content between separator lines
		return c.cleanCommand(clean, prompt)
	}

	// For normal prompts, extract ● responses
	return c.cleanResponse(clean, prompt)
}

func (c *claudeCleaner) cleanCommand(clean string, prompt string) string {
	lines := strings.Split(clean, "\n")
	var result []string
	inContent := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip prompt echo
		if strings.HasPrefix(trimmed, ">") && strings.Contains(line, prompt) {
			continue
		}

		// Separator line marks start of content
		// Pattern: long line of ─ characters (at least 50)
		isSeparator := len(trimmed) > 50 && regexp.MustCompile(`^─+$`).MatchString(trimmed)

		if isSeparator && !inContent {
			// First separator = start of content
			inContent = true
			continue
		}

		// Once we're collecting content, look for end conditions
		if inContent {
			// Another separator = clean end
			if isSeparator {
				break
			}
			// Bottom chrome = end of content area
			if c.isBottomChrome(line) {
				break
			}
			// Skip "dismissed" messages
			if strings.Contains(line, "dismissed") {
				continue
			}
			// Keep all other content
			result = append(result, line)
		}
	}

	// Clean up trailing empty lines
	for len(result) > 0 && strings.TrimSpace(result[len(result)-1]) == "" {
		result = result[:len(result)-1]
	}

	return strings.Join(result, "\n")
}

func (c *claudeCleaner) cleanResponse(clean string, prompt string) string {
	lines := strings.Split(clean, "\n")
	var result []string
	inResponse := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip completely empty lines at start
		if !inResponse && trimmed == "" {
			continue
		}

		// IMPORTANT: Check for response marker BEFORE noise filtering
		// Otherwise lines like "● Hello! I'm Claude Code..." get filtered as noise
		if strings.HasPrefix(trimmed, "●") {
			inResponse = true
			// Remove the ● prefix and keep the content
			content := strings.TrimPrefix(trimmed, "●")
			content = strings.TrimSpace(content)
			if content != "" {
				result = append(result, content)
			}
			continue
		}

		// Always filter thinking indicators and spinners (even during response)
		if c.isThinkingIndicator(line) {
			continue
		}

		// If in response mode and hit bottom chrome, stop collecting
		if inResponse && c.isBottomChrome(line) {
			break
		}

		// Only filter generic UI chrome before response starts
		if !inResponse && c.isGenericNoise(line) {
			continue
		}

		// Skip lines that are just ">" with optional whitespace
		// These are cursor/prompt redraws
		if trimmed == ">" || trimmed == "" {
			continue
		}

		// Skip prompt echo
		if strings.HasPrefix(trimmed, ">") && strings.Contains(line, prompt) {
			continue
		}

		// If we're in response mode, keep non-chrome content
		// Continuation lines (indented content after ●) should be kept
		if inResponse {
			if c.isBottomChrome(line) {
				// Hit bottom chrome, stop collecting
				break
			}
			if trimmed != "" {
				result = append(result, strings.TrimLeft(line, " \t"))
			} else {
				// Keep empty lines within response
				result = append(result, "")
			}
		}
	}

	// Clean up trailing empty lines
	for len(result) > 0 && strings.TrimSpace(result[len(result)-1]) == "" {
		result = result[:len(result)-1]
	}

	return strings.Join(result, "\n")
}

// isThinkingIndicator checks for thinking/status indicators (always filter these)
func (c *claudeCleaner) isThinkingIndicator(line string) bool {
	trimmed := strings.TrimSpace(line)

	// Empty or whitespace-only lines
	if trimmed == "" {
		return true
	}

	// Lines that are just ">" (prompt redraws)
	if trimmed == ">" {
		return true
	}

	// Generic thinking/status indicators:
	// - "(esc to interrupt)" - appears during any processing phase
	// - "∴" - the "Thought for..." indicator
	if strings.Contains(line, "(esc to interrupt)") {
		return true
	}

	// "∴" (three-dot triangle) followed by any text is a thought indicator
	if strings.Contains(line, "∴") {
		return true
	}

	// Animated spinner characters (dot/asterisk morphing animation) + ellipsis
	// Pattern: <animated char> <Word>… (esc to interrupt)
	// These are the thinking status lines
	animatedChars := []rune{'·', '•', '*', '✶', '✻', '✽', '✢', '✺', '✸', '✹', '✦', '✧'}
	for _, r := range animatedChars {
		if strings.HasPrefix(trimmed, string(r)+" ") && strings.Contains(line, "…") {
			return true
		}
	}

	// Short lines with ellipsis are likely status indicators
	if strings.Contains(line, "…") && len(trimmed) < 50 {
		return true
	}

	// Check for progress spinner characters at start of line
	spinners := []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏', '⢀', '⡀', '✓', '⚠'}
	for _, r := range spinners {
		if strings.HasPrefix(trimmed, string(r)) {
			return true
		}
	}

	return false
}

// isGenericNoise checks for startup/UI chrome (only filter before response starts)
func (c *claudeCleaner) isGenericNoise(line string) bool {
	trimmed := strings.TrimSpace(line)

	// Startup/welcome chrome and UI elements
	noisePatterns := []string{
		"https://code.claude.com",
		"Learn more",
		"Press Ctrl-D",
		"WARNING:",
		"By proceeding, you accept",
		"Enter to confirm",
		"Esc to exit",
		"❯ 1.",
		"❯ 2.",
		"In Bypass Permissions mode",
		"Welcome back",
		"Tips for getting started",
		"Recent activity",
		"No recent activity",
		"Run /init to create",
		"Sonnet 4.5",
		"Claude Pro",
		"Claude Code",
		"Note: You have launched",
		"▐▛███▜▌",  // ASCII art logo
		"▝▜█████▛▘",
		"▘▘ ▝▝",
		"Tip: Send messages to Claude",
		"⎿",  // UI decoration character
	}

	for _, pattern := range noisePatterns {
		if strings.Contains(line, pattern) {
			return true
		}
	}

	// Skip lines that are just box drawing characters or separators
	if regexp.MustCompile(`^[─│┌┐└┘├┤┬┴┼═║╔╗╚╝╠╣╦╩╬╭╮╰╯\\s]+$`).MatchString(trimmed) {
		return true
	}

	return false
}

func (c *claudeCleaner) isBottomChrome(line string) bool {
	trimmed := strings.TrimSpace(line)

	// Long separator lines (like the one before bottom chrome)
	if len(trimmed) > 100 && regexp.MustCompile(`^─+$`).MatchString(trimmed) {
		return true
	}

	// Bottom UI chrome patterns
	chromePatterns := []string{
		"bypass permissions on",
		"Thinking on (tab to toggle)",
		"shift+tab to cycle",
		"Auto-update",
		"Auto-updating",
		"claude doctor",
		"npm i -g",
		"⏵⏵",
	}

	for _, pattern := range chromePatterns {
		if strings.Contains(line, pattern) {
			return true
		}
	}

	return false
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
// - No practical way to "load" context from one session into another
// - Context sharing would require either:
//   1. Expensive message replay (burns tokens, slow)
//   2. Lossy summarization (defeats purpose of context sharing)
//   3. File manipulation (risky, undefined behavior)
//
// For session management:
// - Use NewClaudeWithResume(sessionID) to continue a specific session
// - Use NewClaudeWithContinue() to continue most recent session
// - Use ListSessions(cwd) to browse available sessions
// Claude automatically saves conversations to ~/.claude/projects/<dir-path>/<session-id>.jsonl

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
