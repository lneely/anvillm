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

// RolesFS provides 9P filesystem access to agent roles organized by focus areas.
// Roles are markdown files with YAML front-matter defining specialized agent personas.
type RolesFS struct {
	rolesDirs []string
}

// RoleMeta holds parsed YAML front-matter from role markdown files.
type RoleMeta struct {
	Name        string   // Physical filename (without .md)
	DisplayName string   // Name from frontmatter (for discovery)
	FocusAreas  []string
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

// parseRoleFrontMatter extracts focus-areas and description from role markdown YAML front-matter
func parseRoleFrontMatter(rolePath string, derivedFocusAreas []string) (*RoleMeta, error) {
	f, err := os.Open(rolePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	meta := &RoleMeta{
		Name:       strings.TrimSuffix(filepath.Base(rolePath), ".md"),
		Path:       rolePath,
		FocusAreas: derivedFocusAreas,
	}

	scanner := bufio.NewScanner(f)
	inFrontMatter := false
	hasFrontMatterFocusAreas := false

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

		if focusAreas, ok := strings.CutPrefix(line, "focus-areas:"); ok {
			hasFrontMatterFocusAreas = true
			for fa := range strings.SplitSeq(focusAreas, ",") {
				fa = strings.TrimSpace(fa)
				if fa != "" && !slices.Contains(meta.FocusAreas, fa) {
					meta.FocusAreas = append(meta.FocusAreas, fa)
				}
			}
		} else if desc, ok := strings.CutPrefix(line, "description:"); ok {
			meta.Description = strings.TrimSpace(desc)
		} else if name, ok := strings.CutPrefix(line, "name:"); ok {
			meta.DisplayName = strings.TrimSpace(name)
		}
	}

	// If no frontmatter focus-areas and no derived, add "uncategorized"
	if !hasFrontMatterFocusAreas && len(derivedFocusAreas) == 0 {
		meta.FocusAreas = []string{"uncategorized"}
	}

	return meta, nil
}

// scanRolesDir recursively scans a directory for role markdown files
func (r *RolesFS) scanRolesDir(baseDir string, currentPath string, derivedFocusAreas []string) ([]*RoleMeta, error) {
	var roles []*RoleMeta

	entries, err := os.ReadDir(currentPath)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		fullPath := filepath.Join(currentPath, entry.Name())

		if entry.IsDir() {
			// Derive focus area from directory structure
			newDerived := append([]string{}, derivedFocusAreas...)
			newDerived = append(newDerived, entry.Name())
			subRoles, err := r.scanRolesDir(baseDir, fullPath, newDerived)
			if err != nil {
				continue
			}
			roles = append(roles, subRoles...)
		} else if strings.HasSuffix(entry.Name(), ".md") {
			meta, err := parseRoleFrontMatter(fullPath, derivedFocusAreas)
			if err != nil {
				continue
			}
			roles = append(roles, meta)
		}
	}

	return roles, nil
}

// listAllRoles scans all roles directories and returns metadata
func (r *RolesFS) listAllRoles() ([]*RoleMeta, error) {
	seen := make(map[string]*RoleMeta)

	for _, dir := range r.rolesDirs {
		roles, err := r.scanRolesDir(dir, dir, nil)
		if err != nil {
			continue
		}

		for _, role := range roles {
			// Use physical filename (role.Name) as unique key
			// Merge: keep first occurrence, union focus areas
			if existing, exists := seen[role.Name]; exists {
				for _, fa := range role.FocusAreas {
					if !slices.Contains(existing.FocusAreas, fa) {
						existing.FocusAreas = append(existing.FocusAreas, fa)
					}
				}
			} else {
				seen[role.Name] = role
			}
		}
	}

	var result []*RoleMeta
	for _, role := range seen {
		result = append(result, role)
	}
	return result, nil
}

// listFocusAreas returns unique focus area names from all roles
func (r *RolesFS) listFocusAreas() ([]string, error) {
	roles, err := r.listAllRoles()
	if err != nil {
		return nil, err
	}

	focusSet := make(map[string]bool)
	for _, role := range roles {
		for _, fa := range role.FocusAreas {
			focusSet[fa] = true
		}
	}

	var focusAreas []string
	for fa := range focusSet {
		focusAreas = append(focusAreas, fa)
	}
	return focusAreas, nil
}

// listRolesInFocusArea returns roles that have the given focus area
func (r *RolesFS) listRolesInFocusArea(focusArea string) ([]*RoleMeta, error) {
	roles, err := r.listAllRoles()
	if err != nil {
		return nil, err
	}

	var result []*RoleMeta
	for _, role := range roles {
		if slices.Contains(role.FocusAreas, focusArea) {
			result = append(result, role)
		}
	}
	return result, nil
}

// generateHelp creates aggregated index: focus-area/role-name\tdescription
func (r *RolesFS) generateHelp() (string, error) {
	roles, err := r.listAllRoles()
	if err != nil {
		return "", err
	}

	var lines []string
	for _, role := range roles {
		displayName := role.DisplayName
		if displayName == "" {
			displayName = role.Name
		}
		for _, fa := range role.FocusAreas {
			line := fmt.Sprintf("%s/%s\t%s", fa, displayName, role.Description)
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n") + "\n", nil
}

// List returns directory entries for a roles path
func (r *RolesFS) List(path string) ([]plan9.Dir, error) {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")

	// /agent/roles - list focus areas + help file
	if len(parts) == 2 && parts[0] == "agent" && parts[1] == "roles" {
		focusAreas, err := r.listFocusAreas()
		if err != nil {
			return nil, err
		}

		var dirs []plan9.Dir
		dirs = append(dirs, plan9.Dir{
			Name: "help",
			Qid:  plan9.Qid{Type: plan9.QTFILE},
			Mode: 0444,
		})
		for _, fa := range focusAreas {
			dirs = append(dirs, plan9.Dir{
				Name: fa,
				Qid:  plan9.Qid{Type: plan9.QTDIR},
				Mode: plan9.DMDIR | 0555,
			})
		}
		return dirs, nil
	}

	// /agent/roles/<focus-area> - list role files
	if len(parts) == 3 && parts[0] == "agent" && parts[1] == "roles" {
		focusArea := parts[2]
		roles, err := r.listRolesInFocusArea(focusArea)
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

// Read returns file content for a roles path
func (r *RolesFS) Read(path string) ([]byte, error) {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")

	// /agent/roles/help
	if len(parts) == 3 && parts[0] == "agent" && parts[1] == "roles" && parts[2] == "help" {
		help, err := r.generateHelp()
		if err != nil {
			return nil, err
		}
		return []byte(help), nil
	}

	// /agent/roles/<focus-area>/<role-name>.md
	if len(parts) != 4 || parts[0] != "agent" || parts[1] != "roles" {
		return nil, fmt.Errorf("not found")
	}

	focusArea := parts[2]
	fileName := parts[3]

	if !strings.HasSuffix(fileName, ".md") {
		return nil, fmt.Errorf("not a markdown file")
	}

	roles, err := r.listRolesInFocusArea(focusArea)
	if err != nil {
		return nil, err
	}

	for _, role := range roles {
		if role.Name == strings.TrimSuffix(fileName, ".md") {
			return os.ReadFile(role.Path)
		}
	}

	return nil, fmt.Errorf("role not found")
}
