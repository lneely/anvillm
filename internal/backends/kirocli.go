// Package backends provides concrete backend implementations
package backends

import (
	"anvillm/internal/backend"
	"anvillm/internal/backend/tmux"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// NewKiroCLI creates a kiro-cli backend
// Runs with --trust-all-tools because landrun provides the actual sandboxing
func NewKiroCLI() backend.Backend {
	return tmux.New(tmux.Config{
		Name:    "kiro-cli",
		Command: []string{"kiro-cli", "chat", "--trust-all-tools"},
		Environment: map[string]string{
			"TERM":     "xterm-256color",
			"NO_COLOR": "1",
			"COLUMNS":  "999",
		},
		TmuxSize: tmux.TmuxSize{
			Rows: 40,
			Cols: 120,
		},
		StartupTime: 30 * time.Second,
		Detector:    &kiroDetector{},
		Cleaner:     &kiroCleaner{},
		Commands:    &kiroCommands{},
	})
}

// kiroDetector implements pattern detection for kiro-cli
type kiroDetector struct{}

func (d *kiroDetector) IsReady(output string) bool {
	clean := stripANSI(output)
	return strings.Contains(clean, "> ")
}

func (d *kiroDetector) IsComplete(prompt string, output string) bool {
	isCommand := strings.HasPrefix(strings.TrimSpace(prompt), "/")

	if isCommand {
		// Slash commands: wait for next prompt line: [agent] > or !>
		clean := stripANSI(output)
		clean = strings.ReplaceAll(clean, "\r", "")

		// Need substantial content first
		if len(clean) < 200 {
			return false
		}

		// Look for prompt pattern: [something] > or !>
		lines := strings.Split(clean, "\n")
		for i := len(lines) - 1; i >= 0; i-- {
			line := strings.TrimSpace(lines[i])
			// Match either [agent] > or !>
			if (strings.Contains(line, "] ") && strings.Contains(line, ">")) || strings.HasPrefix(line, "!>") {
				return true
			}
		}
		return false
	} else {
		// Normal prompts: wait for "Time:" marker
		return strings.Contains(output, "Time:")
	}
}

// kiroCleaner implements output cleaning for kiro-cli
type kiroCleaner struct{}

func (c *kiroCleaner) Clean(prompt string, rawOutput string) string {
	isCommand := strings.HasPrefix(strings.TrimSpace(prompt), "/")

	if isCommand {
		return c.cleanCommand(rawOutput)
	}
	return c.cleanResponse(rawOutput)
}

func (c *kiroCleaner) cleanResponse(raw string) string {
	// Strip ANSI codes
	clean := stripANSI(raw)
	clean = strings.ReplaceAll(clean, "\r", "\n")

	lines := strings.Split(clean, "\n")
	var result []string
	foundUserPrompt := false
	inResponse := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Look for user prompt: "!>" or "[agent] !>"
		if !foundUserPrompt && strings.Contains(trimmed, "!>") {
			foundUserPrompt = true
			continue
		}

		// Skip echoed user input until we see response marker "> " (but not "!> ")
		if foundUserPrompt && !inResponse && trimmed != "" {
			// Response marker is "> " not preceded by "!"
			idx := strings.Index(trimmed, "> ")
			if idx < 0 || (idx > 0 && trimmed[idx-1] == '!') {
				continue
			}
		}

		// Stop at next prompt containing "!>"
		if foundUserPrompt && inResponse && strings.Contains(trimmed, "!>") {
			break
		}

		// After user prompt, start collecting non-noise content
		if foundUserPrompt && trimmed != "" {
			// Check if line contains actual content (even if it has noise)
			// Important markers: "I will run", "using tool:", "Purpose:", ">", or actual content after spinners
			hasContent := false
			text := trimmed

			// If line contains "Thinking..." followed by content, extract the content
			if strings.Contains(line, "Thinking...") {
				idx := strings.Index(line, "Thinking...")
				afterThinking := strings.TrimSpace(line[idx+11:]) // "Thinking..." is 11 chars
				if afterThinking != "" {
					text = afterThinking
					hasContent = true
				}
			}

			// Check for important markers
			if !hasContent && (strings.Contains(trimmed, "I will run") ||
				strings.Contains(trimmed, "using tool:") ||
				strings.Contains(trimmed, "Purpose:") ||
				strings.Contains(trimmed, ">")) {
				hasContent = true
			}

			// If no content markers and it's noise, skip
			if !hasContent && c.isNoise(line) {
				continue
			}

			// If we got here, it's not pure noise
			if !hasContent {
				text = trimmed
			}

			// Start collecting
			if !inResponse {
				inResponse = true
			}

			// If line has ">", strip it (for assistant response lines)
			if strings.Contains(text, ">") {
				idx := strings.Index(text, ">")
				if idx == 0 || (idx > 0 && text[idx-1] != '!') {
					text = strings.TrimSpace(text[idx+1:])
				}
			}

			if text != "" {
				result = append(result, text)
			}
			continue
		}

		// Before user prompt, skip noise and look for hooks
		if c.isNoise(line) {
			continue
		}

		// Keep hooks lines
		if strings.Contains(line, "hooks finished") {
			result = append(result, trimmed)
			continue
		}
	}

	return strings.Join(result, "\n")
}

func (c *kiroCleaner) cleanCommand(raw string) string {
	clean := stripANSI(raw)
	clean = strings.ReplaceAll(clean, "\r", "\n")

	lines := strings.Split(clean, "\n")
	var result []string
	collecting := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Detect prompt lines: [agent] > or !>
		isPrompt := (strings.Contains(line, "[") && strings.Contains(line, "]") && strings.Contains(line, ">")) ||
			strings.HasPrefix(trimmed, "!>")

		if isPrompt {
			if collecting {
				break // Second prompt = end
			}
			continue
		}

		// Check for summary line (contains "> ") - might be mixed with noise
		if idx := strings.Index(trimmed, "> "); idx >= 0 {
			// Extract everything from "> " onwards
			summary := strings.TrimSpace(trimmed[idx:])
			collecting = true
			result = append(result, summary)
			continue
		}

		// Skip command echo and noise
		if strings.HasPrefix(trimmed, "/") ||
		   strings.Contains(trimmed, "Switching to") ||
		   c.isNoise(line) {
			continue
		}

		collecting = true
		result = append(result, trimmed)
	}

	return strings.Join(result, "\n")
}

func (c *kiroCleaner) isNoise(line string) bool {
	trimmed := strings.TrimSpace(line)

	// Check for noise patterns
	noisePatterns := []string{
		"Thinking...",
		"▸ Credits:",
		"▸ Time:",
	}

	for _, pattern := range noisePatterns {
		if strings.Contains(line, pattern) {
			return true
		}
	}

	// Check for spinner characters
	spinners := []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏', '⢀', '⡀', '✓'}
	for _, r := range spinners {
		if strings.HasPrefix(trimmed, string(r)) {
			return true
		}
	}

	// Check for prompt lines
	if strings.Contains(line, "[") && strings.Contains(line, "]") && strings.Contains(line, ">") {
		return true
	}

	return false
}

// kiroCommands implements command handling for kiro-cli
type kiroCommands struct{}

func (h *kiroCommands) IsSupported(command string) bool {
	supported := []string{"/chat load", "/chat save", "/help", "/agent list"}

	for _, cmd := range supported {
		if strings.HasPrefix(command, cmd) {
			return true
		}
	}
	return false
}

func (h *kiroCommands) Execute(ctx context.Context, command string) (string, error) {
	// For kiro-cli, commands are executed by sending them to the PTY
	// This is handled by the Session.Send() method
	// We just validate the command here
	if !h.IsSupported(command) {
		return "", fmt.Errorf("unsupported command")
	}
	return "", fmt.Errorf("execute called directly - use Send() instead")
}

// Helper functions

func stripANSI(s string) string {
	ansiRegex := regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]`)
	return ansiRegex.ReplaceAllString(s, "")
}

// SessionsDir returns the path to the sessions storage directory
func SessionsDir() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".config", "anvillm", "sessions")
	os.MkdirAll(dir, 0755)
	return dir
}

// GenerateSmartFilename creates a descriptive filename from saved conversation JSON
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

	// If no prompt in JSON, try to extract from window body
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

		stopWords := []string{
			"a", "an", "and", "are", "as", "at", "be", "by", "can", "do",
			"for", "from", "has", "have", "he", "her", "his", "how", "if",
			"in", "into", "is", "it", "its", "just", "me", "my", "no", "not",
			"of", "on", "or", "our", "out", "so", "some", "than", "that",
			"the", "their", "them", "then", "there", "these", "they", "this",
			"to", "up", "us", "was", "we", "what", "when", "which", "who",
			"will", "with", "would", "you", "your",
		}

		for _, w := range words {
			if len(w) > 2 && !contains(stopWords, w) {
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

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// SendCtl is a helper to send a control command to kiro-cli
func SendCtl(sess backend.Session, cmd string) error {
	_, err := sess.Send(context.Background(), cmd)
	return err
}
