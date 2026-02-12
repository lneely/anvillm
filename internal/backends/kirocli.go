// Package backends provides concrete backend implementations
package backends

import (
	"anvillm/internal/backend"
	"anvillm/internal/backend/tmux"
	"context"
	"fmt"
	"os"
	"os/exec"
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
		StartupTime:    30 * time.Second,
		Commands:       &kiroCommands{},
		StateInspector: &kiroStateInspector{},
	})
}

// kiroStateInspector implements StateInspector for kiro-cli
type kiroStateInspector struct{}

func (i *kiroStateInspector) IsBusy(panePID int) bool {
	// Find kiro-cli-chat PID in the process tree: pane -> bash -> kiro-cli -> kiro-cli-chat
	chatPID := tmux.FindKiroChatPID(panePID)
	if chatPID == 0 {
		return false
	}
	// Check if kiro-cli-chat has any children (tool executions)
	cmd := exec.Command("pgrep", "-P", fmt.Sprintf("%d", chatPID))
	return cmd.Run() == nil
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
