package p9

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"9fans.net/go/plan9"
)

// SkillsFS provides virtual filesystem for skills organized by intent
type SkillsFS struct {
	skillsDirs []string
}

// SkillMeta holds parsed YAML front-matter from SKILL.md
type SkillMeta struct {
	Name        string
	Intents     []string
	Description string
	Path        string // directory path
}

// NewSkillsFS creates a new skills filesystem
func NewSkillsFS() *SkillsFS {
	var dirs []string

	// Check ANVILLM_SKILLS_DIR first (colon-separated)
	if envDirs := os.Getenv("ANVILLM_SKILLS_DIR"); envDirs != "" {
		dirs = strings.Split(envDirs, ":")
	} else {
		// Default fallback
		homeDir, _ := os.UserHomeDir()
		dirs = []string{filepath.Join(homeDir, ".config/anvillm/skills")}
	}

	return &SkillsFS{skillsDirs: dirs}
}

// parseSkillFrontMatter extracts intent and description from SKILL.md YAML front-matter
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

		if strings.HasPrefix(line, "intent:") {
			intents := strings.TrimPrefix(line, "intent:")
			for _, i := range strings.Split(intents, ",") {
				i = strings.TrimSpace(i)
				if i != "" {
					meta.Intents = append(meta.Intents, i)
				}
			}
		} else if strings.HasPrefix(line, "description:") {
			meta.Description = strings.TrimSpace(strings.TrimPrefix(line, "description:"))
		}
	}

	return meta, nil
}

// listAllSkills scans all skills directories and returns metadata
func (s *SkillsFS) listAllSkills() ([]*SkillMeta, error) {
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
			skillDir := filepath.Join(dir, entry.Name())
			if _, err := os.Stat(filepath.Join(skillDir, "SKILL.md")); err != nil {
				continue
			}
			meta, err := parseSkillFrontMatter(skillDir)
			if err != nil {
				continue
			}
			skills = append(skills, meta)
		}
	}

	return skills, nil
}

// listIntents returns unique intent names from all skills
func (s *SkillsFS) listIntents() ([]string, error) {
	skills, err := s.listAllSkills()
	if err != nil {
		return nil, err
	}

	intentSet := make(map[string]bool)
	for _, skill := range skills {
		for _, intent := range skill.Intents {
			intentSet[intent] = true
		}
	}

	var intents []string
	for intent := range intentSet {
		intents = append(intents, intent)
	}
	return intents, nil
}

// listSkillsInIntent returns skills that have the given intent
func (s *SkillsFS) listSkillsInIntent(intent string) ([]*SkillMeta, error) {
	skills, err := s.listAllSkills()
	if err != nil {
		return nil, err
	}

	var result []*SkillMeta
	for _, skill := range skills {
		for _, i := range skill.Intents {
			if i == intent {
				result = append(result, skill)
				break
			}
		}
	}
	return result, nil
}

// generateHelp creates aggregated index: intent/skill-name\tdescription
func (s *SkillsFS) generateHelp() (string, error) {
	skills, err := s.listAllSkills()
	if err != nil {
		return "", err
	}

	var lines []string
	for _, skill := range skills {
		for _, intent := range skill.Intents {
			line := fmt.Sprintf("%s/%s\t%s", intent, skill.Name, skill.Description)
			lines = append(lines, line)
		}
		// Skills without intents go under "uncategorized"
		if len(skill.Intents) == 0 {
			line := fmt.Sprintf("uncategorized/%s\t%s", skill.Name, skill.Description)
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n") + "\n", nil
}

// List returns directory entries for a skills path
func (s *SkillsFS) List(path string) ([]plan9.Dir, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")

	// /skills - list intents + help file
	if len(parts) == 2 && parts[0] == "agent" && parts[1] == "skills" {
		intents, err := s.listIntents()
		if err != nil {
			return nil, err
		}

		var dirs []plan9.Dir
		dirs = append(dirs, plan9.Dir{
			Name: "help",
			Qid:  plan9.Qid{Type: plan9.QTFILE},
			Mode: 0444,
		})
		for _, intent := range intents {
			dirs = append(dirs, plan9.Dir{
				Name: intent,
				Qid:  plan9.Qid{Type: plan9.QTDIR},
				Mode: plan9.DMDIR | 0555,
			})
		}
		return dirs, nil
	}

	// /skills/<intent> - list skill directories
	if len(parts) == 3 && parts[0] == "agent" && parts[1] == "skills" {
		intent := parts[2]
		skills, err := s.listSkillsInIntent(intent)
		if err != nil {
			return nil, err
		}

		var dirs []plan9.Dir
		for _, skill := range skills {
			dirs = append(dirs, plan9.Dir{
				Name: skill.Name,
				Qid:  plan9.Qid{Type: plan9.QTDIR},
				Mode: plan9.DMDIR | 0555,
			})
		}
		return dirs, nil
	}

	// /skills/<intent>/<skillname> - list files in skill directory
	if len(parts) == 4 && parts[0] == "agent" && parts[1] == "skills" {
		intent := parts[2]
		skillName := parts[3]

		skills, err := s.listSkillsInIntent(intent)
		if err != nil {
			return nil, err
		}

		for _, skill := range skills {
			if skill.Name == skillName {
				entries, err := os.ReadDir(skill.Path)
				if err != nil {
					return nil, err
				}

				var dirs []plan9.Dir
				for _, entry := range entries {
					mode := plan9.Perm(0444)
					qtype := uint8(plan9.QTFILE)
					if entry.IsDir() {
						mode = plan9.DMDIR | 0555
						qtype = uint8(plan9.QTDIR)
					}
					dirs = append(dirs, plan9.Dir{
						Name: entry.Name(),
						Qid:  plan9.Qid{Type: qtype},
						Mode: mode,
					})
				}
				return dirs, nil
			}
		}
	}

	return nil, fmt.Errorf("not found")
}

// Read returns file content for a skills path
func (s *SkillsFS) Read(path string) ([]byte, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")

	// /skills/help
	if len(parts) == 2 && parts[0] == "skills" && parts[1] == "help" {
		help, err := s.generateHelp()
		if err != nil {
			return nil, err
		}
		return []byte(help), nil
	}

	// /skills/<intent>/<skillname>/<file>
	if len(parts) < 4 || parts[0] != "skills" {
		return nil, fmt.Errorf("not found")
	}

	intent := parts[1]
	skillName := parts[2]
	fileName := strings.Join(parts[3:], "/")

	// Prevent path traversal
	if strings.Contains(fileName, "..") {
		return nil, fmt.Errorf("invalid path")
	}

	skills, err := s.listSkillsInIntent(intent)
	if err != nil {
		return nil, err
	}

	for _, skill := range skills {
		if skill.Name == skillName {
			filePath := filepath.Join(skill.Path, fileName)
			return os.ReadFile(filePath)
		}
	}

	return nil, fmt.Errorf("skill not found")
}
