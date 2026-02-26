package p9

import (
	"fmt"
	"os"
	"strings"

	"9fans.net/go/plan9"
)

type ToolsFS struct {
	tools []Tool
}

type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema InputSchema `json:"inputSchema"`
}

type InputSchema struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties"`
	Required   []string            `json:"required,omitempty"`
}

type Property struct {
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Items       *Items   `json:"items,omitempty"`
	Enum        []string `json:"enum,omitempty"`
}

type Items struct {
	Type string `json:"type"`
}

func NewToolsFS(tools []Tool) *ToolsFS {
	return &ToolsFS{tools: tools}
}

func (t *ToolsFS) List(path string) ([]plan9.Dir, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	
	if len(parts) == 2 && parts[0] == "agent" && parts[1] == "tools" {
		return []plan9.Dir{
			{Name: "anvilmcp", Qid: plan9.Qid{Type: plan9.QTDIR}, Mode: plan9.DMDIR | 0555},
		}, nil
	}
	
	if len(parts) == 3 && parts[0] == "agent" && parts[1] == "tools" && parts[2] == "anvilmcp" {
		// Dynamically list .sh files from ~/.config/anvillm/mcptools/
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		
		mcptoolsDir := fmt.Sprintf("%s/.config/anvillm/mcptools", homeDir)
		entries, err := os.ReadDir(mcptoolsDir)
		if err != nil {
			return nil, err
		}
		
		var dirs []plan9.Dir
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sh") {
				continue
			}
			dirs = append(dirs, plan9.Dir{
				Name: entry.Name(),
				Qid:  plan9.Qid{Type: plan9.QTFILE},
				Mode: 0555,
			})
		}
		return dirs, nil
	}
	
	return nil, fmt.Errorf("not found")
}

func (t *ToolsFS) Read(path string) ([]byte, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	
	if len(parts) != 3 || parts[0] != "tools" || parts[1] != "anvilmcp" {
		return nil, fmt.Errorf("not found")
	}
	
	filename := parts[2]
	if !strings.HasSuffix(filename, ".sh") {
		return nil, fmt.Errorf("not a shell script")
	}
	
	// Prevent path traversal
	if strings.Contains(filename, "/") || strings.Contains(filename, "..") {
		return nil, fmt.Errorf("invalid filename")
	}
	
	// Read directly from disk
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	
	scriptPath := fmt.Sprintf("%s/.config/anvillm/mcptools/%s", homeDir, filename)
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		return nil, fmt.Errorf("script not found")
	}
	
	return content, nil
}
