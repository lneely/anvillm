package backends

import (
	"anvillm/internal/backend"
	"anvillm/internal/backend/tmux"
	"os"

	"9fans.net/go/plan9/client"
)

// NewOllie creates an ollie-backed agent backend.
func NewOllie(nsSuffix string) backend.Backend {
	model := os.Getenv("OLLIE_MODEL")
	if model == "" {
		model = os.Getenv("ANVILLM_OLLAMA_MODEL")
	}
	if model == "" {
		model = "qwen3:8b"
	}

	return tmux.New(tmux.Config{
		Name:    "ollie",
		Command: []string{"ollie", model},
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
		StateInspector: &ollieStateInspector{},
		NsSuffix:       nsSuffix,
	})
}

// ollieStateInspector implements StateInspector for ollie.
type ollieStateInspector struct{}

func (i *ollieStateInspector) IsBusy(panePID int) bool {
	// Process tree: pane -> bash -> ollie
	// Hooks (userPromptSubmit/stop) are the primary state signal;
	// this is a fallback that checks if ollie has active child processes.
	olliePID := tmux.FindChildByName(panePID, "ollie")
	if olliePID == 0 {
		return false
	}
	return tmux.FindChildByName(olliePID, "sh") != 0
}
