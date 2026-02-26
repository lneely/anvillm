package p9

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"9fans.net/go/plan9"
)

type ToolsFS struct {
	tools    []Tool
	toolsDir string
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

// ToolMeta holds parsed front-matter from a tool script
type ToolMeta struct {
	Name         string
	Capabilities []string
	Description  string
	Path         string
}

func NewToolsFS(tools []Tool) *ToolsFS {
	homeDir, _ := os.UserHomeDir()
	return &ToolsFS{
		tools:    tools,
		toolsDir: filepath.Join(homeDir, ".config/anvillm/mcptools"),
	}
}

// parseFrontMatter extracts capabilities and description from script comments
func parseFrontMatter(path string) (*ToolMeta, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	meta := &ToolMeta{
		Name: filepath.Base(path),
		Path: path,
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "#") {
			break
		}
		line = strings.TrimPrefix(line, "#")
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "capabilities:") {
			caps := strings.TrimPrefix(line, "capabilities:")
			for _, c := range strings.Split(caps, ",") {
				c = strings.TrimSpace(c)
				if c != "" {
					meta.Capabilities = append(meta.Capabilities, c)
				}
			}
		} else if strings.HasPrefix(line, "description:") {
			meta.Description = strings.TrimSpace(strings.TrimPrefix(line, "description:"))
		}
	}
	return meta, nil
}

// listAllTools scans toolsDir and returns metadata for all .sh files
func (t *ToolsFS) listAllTools() ([]*ToolMeta, error) {
	entries, err := os.ReadDir(t.toolsDir)
	if err != nil {
		return nil, err
	}

	var tools []*ToolMeta
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sh") {
			continue
		}
		meta, err := parseFrontMatter(filepath.Join(t.toolsDir, entry.Name()))
		if err != nil {
			continue
		}
		tools = append(tools, meta)
	}
	return tools, nil
}

// listCapabilities returns unique capability names from all tools
func (t *ToolsFS) listCapabilities() ([]string, error) {
	tools, err := t.listAllTools()
	if err != nil {
		return nil, err
	}

	capSet := make(map[string]bool)
	for _, tool := range tools {
		for _, cap := range tool.Capabilities {
			capSet[cap] = true
		}
	}

	var caps []string
	for cap := range capSet {
		caps = append(caps, cap)
	}
	return caps, nil
}

// listToolsInCapability returns tools that have the given capability
func (t *ToolsFS) listToolsInCapability(capability string) ([]*ToolMeta, error) {
	tools, err := t.listAllTools()
	if err != nil {
		return nil, err
	}

	var result []*ToolMeta
	for _, tool := range tools {
		for _, cap := range tool.Capabilities {
			if cap == capability {
				result = append(result, tool)
				break
			}
		}
	}
	return result, nil
}

// generateHelp creates aggregated one-liner index of all tools
func (t *ToolsFS) generateHelp() (string, error) {
	tools, err := t.listAllTools()
	if err != nil {
		return "", err
	}

	var lines []string
	for _, tool := range tools {
		caps := strings.Join(tool.Capabilities, ",")
		if caps == "" {
			caps = "uncategorized"
		}
		line := fmt.Sprintf("%s/%s\t%s", caps, tool.Name, tool.Description)
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n") + "\n", nil
}

func (t *ToolsFS) List(path string) ([]plan9.Dir, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")

	// /tools - list capabilities + help file
	if len(parts) == 2 && parts[0] == "agent" && parts[1] == "tools" {
		caps, err := t.listCapabilities()
		if err != nil {
			return nil, err
		}

		var dirs []plan9.Dir
		// Add help file
		dirs = append(dirs, plan9.Dir{
			Name: "help",
			Qid:  plan9.Qid{Type: plan9.QTFILE},
			Mode: 0444,
		})
		// Add capability directories
		for _, cap := range caps {
			dirs = append(dirs, plan9.Dir{
				Name: cap,
				Qid:  plan9.Qid{Type: plan9.QTDIR},
				Mode: plan9.DMDIR | 0555,
			})
		}
		return dirs, nil
	}

	// /tools/<capability> - list tools in that capability
	if len(parts) == 3 && parts[0] == "agent" && parts[1] == "tools" {
		capability := parts[2]
		tools, err := t.listToolsInCapability(capability)
		if err != nil {
			return nil, err
		}

		var dirs []plan9.Dir
		for _, tool := range tools {
			dirs = append(dirs, plan9.Dir{
				Name: tool.Name,
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

	// /tools/help - aggregated index
	if len(parts) == 2 && parts[0] == "tools" && parts[1] == "help" {
		help, err := t.generateHelp()
		if err != nil {
			return nil, err
		}
		return []byte(help), nil
	}

	// /tools/<capability>/<tool.sh> - read tool script
	if len(parts) != 3 || parts[0] != "tools" {
		return nil, fmt.Errorf("not found")
	}

	capability := parts[1]
	filename := parts[2]

	if !strings.HasSuffix(filename, ".sh") {
		return nil, fmt.Errorf("not a shell script")
	}

	// Prevent path traversal
	if strings.Contains(filename, "/") || strings.Contains(filename, "..") {
		return nil, fmt.Errorf("invalid filename")
	}

	// Verify tool has this capability
	tools, err := t.listToolsInCapability(capability)
	if err != nil {
		return nil, err
	}

	for _, tool := range tools {
		if tool.Name == filename {
			return os.ReadFile(tool.Path)
		}
	}

	return nil, fmt.Errorf("tool not found in capability")
}
