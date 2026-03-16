// Package config provides global configuration loaded once at startup.
package config

import (
	"os"
	"path/filepath"
	"strings"
)

var (
	// ProjectDirs are valid parent directories for beads mounts.
	// Mounts must be exactly 1 level deep from one of these.
	// Set via ANVILLM_PROJECT_DIRS (colon-separated), defaults to ~/src:~/prj.
	ProjectDirs []string
)

func init() {
	home := os.Getenv("HOME")
	projectDirsStr := os.Getenv("ANVILLM_PROJECT_DIRS")
	if projectDirsStr == "" {
		projectDirsStr = home + "/src:" + home + "/prj"
	}
	for _, dir := range strings.Split(projectDirsStr, ":") {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		if strings.HasPrefix(dir, "~/") {
			dir = filepath.Join(home, dir[2:])
		}
		ProjectDirs = append(ProjectDirs, dir)
	}
}
