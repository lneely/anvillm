// Package backends provides concrete backend implementations
package backends

import (
	"acme-q/internal/backend"
	"acme-q/internal/backend/pty"
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// NewClaude creates a claude CLI backend
func NewClaude() backend.Backend {
	return pty.New(pty.Config{
		Name:    "claude",
		Command: []string{"claude", "--permission-mode=bypassPermissions"},
		Environment: map[string]string{
			"TERM":     "xterm-256color",
			"NO_COLOR": "1",
		},
		PTYSize: pty.PTYSize{
			Rows: 40,
			Cols: 120,
		},
		StartupTime:    45 * time.Second,
		Detector:       &claudeDetector{},
		Cleaner:        &claudeCleaner{},
		Commands:       &claudeCommands{},
		StartupHandler: &claudeStartupHandler{},
	})
}

// claudeStartupHandler handles the startup confirmation dialogs
type claudeStartupHandler struct {
	acceptedBypassWarning bool
	acceptedWorkspaceTrust bool
}

func (h *claudeStartupHandler) HandleStartup(output string) (data []byte, done bool) {
	clean := stripANSI(output)

	// First dialog: Bypass permissions warning
	if !h.acceptedBypassWarning && strings.Contains(clean, "WARNING: Claude Code running in Bypass Permissions mode") {
		if strings.Contains(clean, "Yes, I accept") {
			// Dialog is shown, send "2" + Enter to select "Yes, I accept"
			// Or just press Enter if option 2 is already selected
			// Looking at the dialog format, option 1 is default, so we need to select 2
			h.acceptedBypassWarning = true
			return []byte("2\r"), false
		}
	}

	// Second dialog: Workspace trust
	if h.acceptedBypassWarning && !h.acceptedWorkspaceTrust {
		if strings.Contains(clean, "Do you trust the files in this folder") {
			if strings.Contains(clean, "Yes, proceed") {
				// Send "1" + Enter to select "Yes, proceed"
				h.acceptedWorkspaceTrust = true
				return []byte("1\r"), true // Done after this
			}
		}
	}

	// If we've accepted both, we're done
	if h.acceptedBypassWarning && h.acceptedWorkspaceTrust {
		return nil, true
	}

	// No action needed yet
	return nil, false
}

// claudeDetector implements pattern detection for claude CLI
type claudeDetector struct{}

func (d *claudeDetector) IsReady(output string) bool {
	clean := stripANSI(output)

	// Claude CLI is ready after startup when:
	// 1. We've passed all the dialogs
	// 2. And the prompt is ready for input

	// After startup dialogs, claude might show a welcome message or just be silent
	// We'll consider it ready if we see certain patterns or if there's been
	// substantial output after the dialogs

	// If we see any greeting or welcoming text, consider it ready
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

	// If we've passed dialogs and there's some output, consider ready
	// (The startup handler will have reset output after dialogs)
	if len(clean) > 50 {
		return true
	}

	// If output is minimal but doesn't contain dialog text, likely ready
	if len(clean) > 0 &&
	   !strings.Contains(clean, "WARNING:") &&
	   !strings.Contains(clean, "Do you trust") {
		return true
	}

	return false
}

func (d *claudeDetector) IsComplete(prompt string, output string) bool {
	clean := stripANSI(output)
	clean = strings.ReplaceAll(clean, "\r", "")

	// Claude CLI typically outputs responses and then goes silent
	// We'll look for patterns that indicate completion:

	// 1. If the output is very short (< 100 chars), it's not complete
	if len(clean) < 100 {
		return false
	}

	// 2. For slash commands (if claude supports them), wait for next prompt
	if strings.HasPrefix(strings.TrimSpace(prompt), "/") {
		// Look for command completion patterns
		// This is a placeholder - adjust based on actual claude CLI behavior
		return strings.Contains(clean, "Command completed") || len(clean) > 1000
	}

	// 3. For normal prompts, claude CLI might have different completion markers
	// Common patterns:
	// - Empty lines at the end
	// - Specific markers
	// - Or just wait for substantial output and a pause (handled by timeout)

	// Check if output seems complete (ends with newlines and has substantial content)
	lines := strings.Split(clean, "\n")
	if len(lines) < 5 {
		return false
	}

	// Consider complete if we have substantial output
	// The PTY backend will handle the timeout and draining
	return len(clean) > 200
}

// claudeCleaner implements output cleaning for claude CLI
type claudeCleaner struct{}

func (c *claudeCleaner) Clean(prompt string, rawOutput string) string {
	// Strip ANSI codes
	clean := stripANSI(rawOutput)
	clean = strings.ReplaceAll(clean, "\r", "\n")

	lines := strings.Split(clean, "\n")
	var result []string
	seenResponse := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip empty lines at the start
		if !seenResponse && trimmed == "" {
			continue
		}

		// Skip noise patterns
		if c.isNoise(line) {
			continue
		}

		// Skip the echo of the prompt
		if strings.TrimSpace(line) == strings.TrimSpace(prompt) {
			continue
		}

		if trimmed != "" {
			seenResponse = true
			result = append(result, line)
		} else if seenResponse {
			// Keep empty lines within the response
			result = append(result, "")
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

	// Noise patterns specific to claude CLI
	noisePatterns := []string{
		"WARNING: Claude Code running in Bypass Permissions mode",
		"In Bypass Permissions mode",
		"This mode should only be used",
		"By proceeding, you accept",
		"https://code.claude.com",
		"Learn more",
		"Yes, I accept",
		"No, exit",
		"Enter to confirm",
		"Esc to exit",
		"Press Ctrl-D",
	}

	for _, pattern := range noisePatterns {
		if strings.Contains(line, pattern) {
			return true
		}
	}

	// Check for spinner/loading characters
	spinners := []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏', '⢀', '⡀', '✓', '⚠'}
	for _, r := range spinners {
		if strings.HasPrefix(trimmed, string(r)) {
			return true
		}
	}

	// Skip lines that are just box drawing characters or separators
	if regexp.MustCompile(`^[─│┌┐└┘├┤┬┴┼═║╔╗╚╝╠╣╦╩╬]+$`).MatchString(trimmed) {
		return true
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
	// For claude CLI, commands are executed by sending them to the PTY
	// This is handled by the Session.Send() method
	// We just validate the command here
	if !h.IsSupported(command) {
		return "", fmt.Errorf("unsupported command")
	}
	return "", fmt.Errorf("execute called directly - use Send() instead")
}
