// Package backends provides concrete backend implementations
package backends

import (
	"acme-q/internal/backend"
	"acme-q/internal/backend/tmux"
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// NewClaude creates a claude CLI backend
//
// Uses tmux backend to handle interactive TUI dialogs programmatically.
// The startup handler automatically accepts the --dangerously-skip-permissions dialog.
func NewClaude() backend.Backend {
	return tmux.New(tmux.Config{
		Name:    "claude",
		Command: []string{"claude", "--dangerously-skip-permissions"},
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

	lines := strings.Split(clean, "\n")
	var result []string
	inResponse := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip completely empty lines at start
		if !inResponse && trimmed == "" {
			continue
		}

		// Skip noise (includes thinking indicators, startup chrome, etc.)
		if c.isNoise(line) {
			// If we hit bottom chrome after response started, stop
			if inResponse && c.isBottomChrome(line) {
				break
			}
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

		// Look for actual response content
		// Claude responses start with ● (bullet)
		// Tool usage also uses ●
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

func (c *claudeCleaner) isNoise(line string) bool {
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

	// Check for progress spinner characters at start of line
	spinners := []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏', '⢀', '⡀', '✓', '⚠'}
	for _, r := range spinners {
		if strings.HasPrefix(trimmed, string(r)) {
			return true
		}
	}

	// Skip lines that are just box drawing characters or separators
	if regexp.MustCompile(`^[─│┌┐└┘├┤┬┴┼═║╔╗╚╝╠╣╦╩╬╭╮╰╯\s]+$`).MatchString(trimmed) {
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

// claudeCommands implements command handling for claude CLI
type claudeCommands struct{}

func (h *claudeCommands) IsSupported(command string) bool {
	// Claude CLI supports various slash commands
	// This is a placeholder - adjust based on actual claude CLI capabilities
	supported := []string{"/help", "/clear", "/model", "/settings"}

	for _, cmd := range supported {
		if strings.HasPrefix(command, cmd) {
			return true
		}
	}
	return false
}

func (h *claudeCommands) Execute(ctx context.Context, command string) (string, error) {
	// For claude CLI, commands are executed by sending them to the session
	// This is handled by the Session.Send() method
	// We just validate the command here
	if !h.IsSupported(command) {
		return "", fmt.Errorf("unsupported command")
	}
	return "", fmt.Errorf("execute called directly - use Send() instead")
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
