package p9

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"9fans.net/go/plan9"
)

// ToolsFS provides 9P filesystem access to agent tools organized by capability level.
// Tools are shell scripts with YAML front-matter defining their interface.
type ToolsFS struct {
	tools    []Tool
	toolsDir string
}

// Tool represents an agent tool with its MCP-compatible schema.
type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema InputSchema `json:"inputSchema"`
}

// InputSchema defines the JSON schema for tool input parameters.
type InputSchema struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties"`
	Required   []string            `json:"required,omitempty"`
}

// Property defines a single input parameter schema.
type Property struct {
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Items       *Items   `json:"items,omitempty"`
	Enum        []string `json:"enum,omitempty"`
}

// Items defines the schema for array item types.
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
		if comment, ok := strings.CutPrefix(line, "#"); ok {
			line = strings.TrimSpace(comment)
		} else {
			break
		}

		if caps, ok := strings.CutPrefix(line, "capabilities:"); ok {
			for c := range strings.SplitSeq(caps, ",") {
				c = strings.TrimSpace(c)
				if c != "" {
					meta.Capabilities = append(meta.Capabilities, c)
				}
			}
		} else if desc, ok := strings.CutPrefix(line, "description:"); ok {
			meta.Description = strings.TrimSpace(desc)
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
		if slices.Contains(tool.Capabilities, capability) {
			result = append(result, tool)
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
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")

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
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")

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
	cleanName := filepath.Clean(filename)
	if filepath.IsAbs(cleanName) {
		return nil, fmt.Errorf("invalid filename")
	}

	// Verify tool has this capability
	tools, err := t.listToolsInCapability(capability)
	if err != nil {
		return nil, err
	}

	for _, tool := range tools {
		if tool.Name == filename {
			// Verify path stays within toolsDir
			relPath, err := filepath.Rel(t.toolsDir, tool.Path)
			if err != nil || strings.HasPrefix(relPath, "..") {
				return nil, fmt.Errorf("invalid tool path")
			}
			return os.ReadFile(tool.Path)
		}
	}

	return nil, fmt.Errorf("tool not found in capability")
}
