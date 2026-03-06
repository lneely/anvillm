package p9

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"9fans.net/go/plan9"
)

// RolesFS provides a flat virtual filesystem for roles.
// Roles are discovered from configured directories and exposed as <name>.md files.
// The listing is the union of all roles directories, sorted by name.
type RolesFS struct {
	rolesDirs []string
}

// RoleMeta holds parsed YAML front-matter from role markdown files.
type RoleMeta struct {
	Name        string
	Description string
	Path        string // file path
}

// NewRolesFS creates a new roles filesystem handler.
// It discovers roles from ANVILLM_ROLES_DIR environment variable or default locations.
func NewRolesFS() *RolesFS {
	var dirs []string

	// Check ANVILLM_ROLES_DIR first (colon-separated)
	if envDirs := os.Getenv("ANVILLM_ROLES_DIR"); envDirs != "" {
		dirs = strings.Split(envDirs, ":")
	} else {
		// Default: Claude, Kiro, AnviLLM config directories
		homeDir, _ := os.UserHomeDir()
		var defaults []string
		if claudeDir := os.Getenv("CLAUDE_CONFIG_DIR"); claudeDir != "" {
			defaults = append(defaults, filepath.Join(claudeDir, "roles"))
		}
		defaults = append(defaults,
			filepath.Join(homeDir, ".kiro/roles"),
			filepath.Join(homeDir, ".config/anvillm/roles"),
		)
		dirs = defaults
	}

	return &RolesFS{rolesDirs: dirs}
}

// parseRoleFrontMatter extracts description from role markdown YAML front-matter.
func parseRoleFrontMatter(rolePath string) (*RoleMeta, error) {
	f, err := os.Open(rolePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	meta := &RoleMeta{
		Name: strings.TrimSuffix(filepath.Base(rolePath), ".md"),
		Path: rolePath,
	}

	scanner := bufio.NewScanner(f)
	inFrontMatter := false
	hasDescription := false

	for scanner.Scan() {
		line := scanner.Text()

		if line == "---" {
			if !inFrontMatter {
				inFrontMatter = true
				continue
			} else {
				break // end of front-matter
			}
		}

		if !inFrontMatter {
			continue
		}

		if desc, ok := strings.CutPrefix(line, "description:"); ok {
			meta.Description = strings.TrimSpace(desc)
			hasDescription = true
		}
	}

	// If missing description, exclude from list
	if !hasDescription {
		return nil, fmt.Errorf("missing description")
	}

	return meta, nil
}

// listAllRoles scans all roles directories and returns deduplicated metadata,
// with the first occurrence (by rolesDirs order) winning on name collision.
func (r *RolesFS) listAllRoles() ([]*RoleMeta, error) {
	seen := make(map[string]bool)
	var roles []*RoleMeta

	for _, dir := range r.rolesDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			name := strings.TrimSuffix(entry.Name(), ".md")
			if seen[name] {
				continue
			}
			rolePath := filepath.Join(dir, entry.Name())
			meta, err := parseRoleFrontMatter(rolePath)
			if err != nil {
				continue
			}
			seen[name] = true
			roles = append(roles, meta)
		}
	}

	sort.Slice(roles, func(i, j int) bool { return roles[i].Name < roles[j].Name })
	return roles, nil
}

// List returns directory entries for the flat roles root: agent/roles → <name>.md files.
func (r *RolesFS) List(path string) ([]plan9.Dir, error) {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")

	if len(parts) == 2 && parts[0] == "agent" && parts[1] == "roles" {
		roles, err := r.listAllRoles()
		if err != nil {
			return nil, err
		}
		var dirs []plan9.Dir
		for _, role := range roles {
			dirs = append(dirs, plan9.Dir{
				Name: role.Name + ".md",
				Qid:  plan9.Qid{Type: plan9.QTFILE},
				Mode: 0444,
			})
		}
		return dirs, nil
	}

	return nil, fmt.Errorf("not found")
}

// Read returns file content for agent/roles/<name>.md.
func (r *RolesFS) Read(path string) ([]byte, error) {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")

	// agent/roles/<name>.md
	if len(parts) == 3 && parts[0] == "agent" && parts[1] == "roles" {
		fileName := parts[2]
		if !strings.HasSuffix(fileName, ".md") {
			return nil, fmt.Errorf("not found")
		}
		roleName := strings.TrimSuffix(fileName, ".md")

		roles, err := r.listAllRoles()
		if err != nil {
			return nil, err
		}
		for _, role := range roles {
			if role.Name == roleName {
				return os.ReadFile(role.Path)
			}
		}
		return nil, fmt.Errorf("role not found: %s", roleName)
	}

	return nil, fmt.Errorf("not found")
}

// ReadRole reads a role by name, searching all roles directories.
func (r *RolesFS) ReadRole(roleName string) (string, error) {
	roles, err := r.listAllRoles()
	if err != nil {
		return "", err
	}

	for _, role := range roles {
		if role.Name == roleName {
			content, err := os.ReadFile(role.Path)
			if err != nil {
				return "", err
			}
			return string(content), nil
		}
	}

	return "", fmt.Errorf("role %s not found", roleName)
}
