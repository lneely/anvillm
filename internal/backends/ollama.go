package backends

import (
	"anvillm/internal/backend"
	"anvillm/internal/backend/tmux"
	"fmt"
	"os"
	"os/exec"
	"time"

	"9fans.net/go/plan9/client"
)

// NewOllama creates an ollama backend using mcphost
func NewOllama(nsSuffix string) backend.Backend {
	model := os.Getenv("ANVILLM_OLLAMA_MODEL")
	if model == "" {
		model = "qwen3:8b"
	}

	home, _ := os.UserHomeDir()
	configPath := home + "/.config/anvillm/ollama-mcp.json"

	return tmux.New(tmux.Config{
		Name:    "ollama",
		Command: []string{"ollama-mcp-client", configPath, model},
		Environment: map[string]string{
			"TERM":      "xterm-256color",
			"NO_COLOR":  "1",
			"COLUMNS":   "999",
			"NAMESPACE": client.Namespace(),
		},
		TmuxSize: tmux.TmuxSize{
			Rows: 40,
			Cols: 120,
		},
		StartupTime:    30 * time.Second,
		StateInspector: &ollamaStateInspector{},
		NsSuffix:       nsSuffix,
	})
}

// ollamaStateInspector implements StateInspector for mcphost
type ollamaStateInspector struct{}

func (i *ollamaStateInspector) IsBusy(panePID int) bool {
	// Process tree: pane -> bash -> mcphost
	bashPID := tmux.FindChildByName(panePID, "bash")
	if bashPID == 0 {
		return false
	}

	mcphostPID := tmux.FindChildByName(bashPID, "mcphost")
	if mcphostPID == 0 {
		return false
	}

	// Check if mcphost has any children (tool executions)
	cmd := exec.Command("pgrep", "-P", fmt.Sprintf("%d", mcphostPID))
	return cmd.Run() == nil
}
