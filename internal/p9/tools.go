package p9

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"9fans.net/go/plan9"
)

// ToolsFS provides 9P filesystem access to agent tools organized by capability level.
// Tools are shell scripts with YAML front-matter defining their interface.
type ToolsFS struct {
	tools     []Tool
	toolsDirs []string
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
	var dirs []string

	// Check ANVILLM_TOOLS_DIR first (colon-separated)
	if envDirs := os.Getenv("ANVILLM_TOOLS_DIR"); envDirs != "" {
		dirs = strings.Split(envDirs, ":")
	} else {
		// Default: Claude, Kiro, AnviLLM config directories
		homeDir, _ := os.UserHomeDir()
		var defaults []string
		if claudeDir := os.Getenv("CLAUDE_CONFIG_DIR"); claudeDir != "" {
			defaults = append(defaults, filepath.Join(claudeDir, "mcptools"))
		}
		defaults = append(defaults,
			filepath.Join(homeDir, ".kiro/mcptools"),
			filepath.Join(homeDir, ".config/anvillm/mcptools"),
		)
		dirs = defaults
	}

	return &ToolsFS{
		tools:     tools,
		toolsDirs: dirs,
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

// listAllTools scans all tools directories and returns metadata for all .sh files
func (t *ToolsFS) listAllTools() ([]*ToolMeta, error) {
	seen := make(map[string]bool)
	var tools []*ToolMeta

	for _, dir := range t.toolsDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sh") {
				continue
			}
			if seen[entry.Name()] {
				continue
			}
			meta, err := parseFrontMatter(filepath.Join(dir, entry.Name()))
			if err != nil {
				continue
			}
			seen[entry.Name()] = true
			tools = append(tools, meta)
		}
	}
	return tools, nil
}

func (t *ToolsFS) List(path string) ([]plan9.Dir, error) {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")

	// /tools - list all tools flat
	if len(parts) == 2 && parts[0] == "agent" && parts[1] == "tools" {
		tools, err := t.listAllTools()
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

	// /tools/<tool.sh> - read tool script
	if len(parts) != 2 || parts[0] != "tools" {
		return nil, fmt.Errorf("not found")
	}

	filename := parts[1]

	if !strings.HasSuffix(filename, ".sh") {
		return nil, fmt.Errorf("not a shell script")
	}

	// Prevent path traversal
	cleanName := filepath.Clean(filename)
	if filepath.IsAbs(cleanName) {
		return nil, fmt.Errorf("invalid filename")
	}

	tools, err := t.listAllTools()
	if err != nil {
		return nil, err
	}

	for _, tool := range tools {
		if tool.Name == filename {
			// Verify path stays within one of the tools directories
			valid := false
			for _, dir := range t.toolsDirs {
				relPath, err := filepath.Rel(dir, tool.Path)
				if err == nil && !strings.HasPrefix(relPath, "..") {
					valid = true
					break
				}
			}
			if !valid {
				return nil, fmt.Errorf("invalid tool path")
			}
			return os.ReadFile(tool.Path)
		}
	}

	return nil, fmt.Errorf("tool not found")
}
