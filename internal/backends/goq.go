// Package backends provides concrete backend implementations
package backends

import (
	"anvillm/internal/backend"
	"anvillm/internal/backend/tmux"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// NewGoq creates a goq backend
func NewGoq(nsSuffix string) backend.Backend {
	goqCmd := os.Getenv("ANVILLM_GOQ_CMD")
	if goqCmd == "" {
		goqCmd = "goq"
	}
	return tmux.New(tmux.Config{
		Name:    "goq",
		Command: append(strings.Fields(goqCmd), "--agent", "anvillm-agent"),
		Environment: map[string]string{
			"TERM":     "xterm-256color",
			"NO_COLOR": "1",
		},
		TmuxSize: tmux.TmuxSize{
			Rows: 40,
			Cols: 120,
		},
		StateInspector: &goqStateInspector{},
		ClearHandler:   goqClearHandler,
		CompactHandler: goqCompactHandler,
		NsSuffix:       nsSuffix,
	})
}

func goqClearHandler(target string) error {
	return tmux.SendKeysTo(target, "/clear", "C-m")
}

func goqCompactHandler(target string) error {
	return tmux.SendKeysTo(target, "/compact", "C-m")
}

type goqStateInspector struct{}

func (i *goqStateInspector) IsBusy(panePID int) bool {
	bashPID := tmux.FindChildByName(panePID, "bash")
	if bashPID == 0 {
		return false
	}
	goqPID := tmux.FindChildByName(bashPID, "goq")
	if goqPID == 0 {
		return false
	}
	cmd := exec.Command("pgrep", "-P", fmt.Sprintf("%d", goqPID))
	return cmd.Run() == nil
}
