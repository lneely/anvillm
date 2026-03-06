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

// SkillsFS provides a flat virtual filesystem for skills.
// Skills are discovered from configured directories and exposed as <name>.md files.
// The listing is the union of all skills directories, sorted by name.
type SkillsFS struct {
	skillsDirs []string
}

// SkillMeta holds parsed YAML front-matter from SKILL.md files.
type SkillMeta struct {
	Name        string
	Description string
	Path        string // directory path
}

// NewSkillsFS creates a new skills filesystem handler.
// It discovers skills from ANVILLM_SKILLS_DIR environment variable or default locations.
func NewSkillsFS() *SkillsFS {
	var dirs []string

	// Check ANVILLM_SKILLS_DIR first (colon-separated)
	if envDirs := os.Getenv("ANVILLM_SKILLS_DIR"); envDirs != "" {
		dirs = strings.Split(envDirs, ":")
	} else {
		// Default: Claude, Kiro, AnviLLM config directories
		homeDir, _ := os.UserHomeDir()
		var defaults []string
		if claudeDir := os.Getenv("CLAUDE_CONFIG_DIR"); claudeDir != "" {
			defaults = append(defaults, filepath.Join(claudeDir, "skills"))
		}
		defaults = append(defaults,
			filepath.Join(homeDir, ".kiro/skills"),
			filepath.Join(homeDir, ".config/anvillm/skills"),
		)
		dirs = defaults
	}

	return &SkillsFS{skillsDirs: dirs}
}

// parseSkillFrontMatter extracts name and description from SKILL.md YAML front-matter.
func parseSkillFrontMatter(skillDir string) (*SkillMeta, error) {
	skillFile := filepath.Join(skillDir, "SKILL.md")
	f, err := os.Open(skillFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	meta := &SkillMeta{
		Name: filepath.Base(skillDir),
		Path: skillDir,
	}

	scanner := bufio.NewScanner(f)
	inFrontMatter := false
	hasName := false
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

		if name, ok := strings.CutPrefix(line, "name:"); ok {
			meta.Name = strings.TrimSpace(name)
			hasName = true
		} else if desc, ok := strings.CutPrefix(line, "description:"); ok {
			meta.Description = strings.TrimSpace(desc)
			hasDescription = true
		}
	}

	// If missing name or description, return nil to exclude from list
	if !hasName || !hasDescription {
		return nil, fmt.Errorf("missing name or description")
	}

	return meta, nil
}

// listAllSkills scans all skills directories and returns deduplicated metadata,
// with the first occurrence (by skillsDirs order) winning on name collision.
func (s *SkillsFS) listAllSkills() ([]*SkillMeta, error) {
	seen := make(map[string]bool)
	var skills []*SkillMeta

	for _, dir := range s.skillsDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			if seen[entry.Name()] {
				continue
			}
			skillDir := filepath.Join(dir, entry.Name())
			if _, err := os.Stat(filepath.Join(skillDir, "SKILL.md")); err != nil {
				continue
			}
			meta, err := parseSkillFrontMatter(skillDir)
			if err != nil {
				continue
			}
			seen[entry.Name()] = true
			skills = append(skills, meta)
		}
	}

	sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })
	return skills, nil
}

// List returns directory entries for the flat skills root: agent/skills → <name>.md files.
func (s *SkillsFS) List(path string) ([]plan9.Dir, error) {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")

	if len(parts) == 2 && parts[0] == "agent" && parts[1] == "skills" {
		skills, err := s.listAllSkills()
		if err != nil {
			return nil, err
		}
		var dirs []plan9.Dir
		for _, skill := range skills {
			dirs = append(dirs, plan9.Dir{
				Name: skill.Name + ".md",
				Qid:  plan9.Qid{Type: plan9.QTFILE},
				Mode: 0444,
			})
		}
		return dirs, nil
	}

	return nil, fmt.Errorf("not found")
}

// Read returns SKILL.md content for agent/skills/<name>.md.
func (s *SkillsFS) Read(path string) ([]byte, error) {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")

	// agent/skills/<name>.md
	if len(parts) == 3 && parts[0] == "agent" && parts[1] == "skills" {
		fileName := parts[2]
		if !strings.HasSuffix(fileName, ".md") {
			return nil, fmt.Errorf("not found")
		}
		skillName := strings.TrimSuffix(fileName, ".md")

		skills, err := s.listAllSkills()
		if err != nil {
			return nil, err
		}
		for _, skill := range skills {
			if skill.Name == skillName {
				return os.ReadFile(filepath.Join(skill.Path, "SKILL.md"))
			}
		}
		return nil, fmt.Errorf("skill not found: %s", skillName)
	}

	return nil, fmt.Errorf("not found")
}
