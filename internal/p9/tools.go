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

func generateToolWrapper(tool Tool) string {
	// Try to read actual script from installed location
	homeDir, err := os.UserHomeDir()
	if err == nil {
		scriptPath := fmt.Sprintf("%s/.config/anvillm/mcptools/%s.sh", homeDir, tool.Name)
		if content, err := os.ReadFile(scriptPath); err == nil {
			return string(content)
		}
	}
	
	// Fallback: generate documentation
	var sb strings.Builder
	
	sb.WriteString(fmt.Sprintf("#!/bin/bash\n"))
	sb.WriteString(fmt.Sprintf("# %s\n", tool.Name))
	sb.WriteString(fmt.Sprintf("# %s\n#\n", tool.Description))
	
	// Show usage
	sb.WriteString("# Usage:\n")
	params := []string{}
	for name := range tool.InputSchema.Properties {
		params = append(params, fmt.Sprintf("<%s>", name))
	}
	sb.WriteString(fmt.Sprintf("#   %s %s\n#\n", tool.Name, strings.Join(params, " ")))
	
	// Show parameters
	if len(tool.InputSchema.Properties) > 0 {
		sb.WriteString("# Parameters:\n")
		for name, prop := range tool.InputSchema.Properties {
			required := ""
			for _, req := range tool.InputSchema.Required {
				if req == name {
					required = " (required)"
					break
				}
			}
			sb.WriteString(fmt.Sprintf("#   %s: %s%s\n", name, prop.Description, required))
		}
		sb.WriteString("#\n")
	}
	
	// Show example
	sb.WriteString("# Example:\n")
	sb.WriteString(fmt.Sprintf("#   Call via MCP:\n"))
	sb.WriteString(fmt.Sprintf("#   echo '{\"name\":\"%s\",\"arguments\":{", tool.Name))
	first := true
	for name := range tool.InputSchema.Properties {
		if !first {
			sb.WriteString(",")
		}
		sb.WriteString(fmt.Sprintf("\"%s\":\"value\"", name))
		first = false
	}
	sb.WriteString("}}' | 9p write agent/mcp/call\n")
	
	return sb.String()
}

func jsonTypeToTS(prop Property) string {
	switch prop.Type {
	case "string":
		return "string"
	case "integer", "number":
		return "number"
	case "boolean":
		return "boolean"
	case "array":
		if prop.Items != nil {
			return jsonTypeToTS(Property{Type: prop.Items.Type}) + "[]"
		}
		return "any[]"
	case "object":
		return "any"
	default:
		return "any"
	}
}
