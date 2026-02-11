package sandbox

import (
	"fmt"
	"os"
	"strings"
)

// WrapCommand wraps a command with landrun based on the configuration.
// Returns the wrapped command args, or the original command if sandboxing is disabled.
func WrapCommand(cfg *Config, originalCmd []string, cwd string) []string {
	if !cfg.General.Enabled {
		return originalCmd
	}

	if !IsAvailable() {
		if cfg.General.BestEffort {
			// Silently fall back to unsandboxed execution
			return originalCmd
		}
		// Strict mode: would fail, but caller should check IsAvailable() first
		return originalCmd
	}

	home, _ := os.UserHomeDir()
	tmpdir := os.Getenv("TMPDIR")
	if tmpdir == "" {
		tmpdir = "/tmp"
	}

	args := []string{"landrun"}

	// Log level
	if cfg.General.LogLevel != "" {
		args = append(args, "--log-level", cfg.General.LogLevel)
	}

	// Best effort mode
	if cfg.General.BestEffort {
		args = append(args, "--best-effort")
	}

	// Advanced options
	if cfg.Advanced.LDD {
		args = append(args, "--ldd")
	}
	if cfg.Advanced.AddExec {
		args = append(args, "--add-exec")
	}

	// Filesystem permissions - RO (read-only, no execute)
	for _, path := range cfg.Filesystem.RO {
		expanded := expandPath(path, cwd, home, tmpdir)
		if pathExists(expanded) {
			args = append(args, "--ro", expanded)
		}
	}

	// Filesystem permissions - ROX (read-only with execute)
	for _, path := range cfg.Filesystem.ROX {
		expanded := expandPath(path, cwd, home, tmpdir)
		if pathExists(expanded) {
			args = append(args, "--rox", expanded)
		}
	}

	// Filesystem permissions - RW (read-write, no execute)
	for _, path := range cfg.Filesystem.RW {
		expanded := expandPath(path, cwd, home, tmpdir)
		if pathExists(expanded) {
			args = append(args, "--rw", expanded)
		}
	}

	// Filesystem permissions - RWX (read-write with execute)
	for _, path := range cfg.Filesystem.RWX {
		expanded := expandPath(path, cwd, home, tmpdir)
		if pathExists(expanded) {
			args = append(args, "--rwx", expanded)
		}
	}

	// Network permissions
	if cfg.Network.Unrestricted {
		args = append(args, "--unrestricted-network")
	} else if cfg.Network.Enabled {
		for _, port := range cfg.Network.BindTCP {
			args = append(args, "--bind-tcp", port)
		}
		for _, port := range cfg.Network.ConnectTCP {
			args = append(args, "--connect-tcp", port)
		}
	}
	// If network not enabled and not unrestricted, no network flags = no network access

	// Append original command after --
	args = append(args, "--")
	args = append(args, originalCmd...)

	return args
}

// BuildSummary creates a human-readable summary of sandbox settings
func BuildSummary(cfg *Config) string {
	var lines []string

	if !cfg.General.Enabled {
		return "Sandboxing: DISABLED"
	}

	lines = append(lines, "Sandboxing: ENABLED")

	if cfg.General.BestEffort {
		lines = append(lines, "  Mode: Best-effort (fallback if landrun unavailable)")
	} else {
		lines = append(lines, "  Mode: Strict (require landrun)")
	}

	// Filesystem
	lines = append(lines, "")
	lines = append(lines, "Filesystem:")
	if len(cfg.Filesystem.RW) > 0 {
		lines = append(lines, fmt.Sprintf("  Read-Write: %s", strings.Join(cfg.Filesystem.RW, ", ")))
	}
	if len(cfg.Filesystem.RO) > 0 {
		lines = append(lines, fmt.Sprintf("  Read-Only: %s", strings.Join(cfg.Filesystem.RO, ", ")))
	}
	if len(cfg.Filesystem.ROX) > 0 {
		lines = append(lines, fmt.Sprintf("  Read-Execute: %s", strings.Join(cfg.Filesystem.ROX, ", ")))
	}
	if len(cfg.Filesystem.RWX) > 0 {
		lines = append(lines, fmt.Sprintf("  Read-Write-Execute: %s", strings.Join(cfg.Filesystem.RWX, ", ")))
	}

	// Network
	lines = append(lines, "")
	if cfg.Network.Unrestricted {
		lines = append(lines, "Network: UNRESTRICTED")
	} else if cfg.Network.Enabled {
		lines = append(lines, "Network: LIMITED")
		if len(cfg.Network.ConnectTCP) > 0 {
			lines = append(lines, fmt.Sprintf("  Connect TCP: %s", strings.Join(cfg.Network.ConnectTCP, ", ")))
		}
		if len(cfg.Network.BindTCP) > 0 {
			lines = append(lines, fmt.Sprintf("  Bind TCP: %s", strings.Join(cfg.Network.BindTCP, ", ")))
		}
	} else {
		lines = append(lines, "Network: BLOCKED")
	}

	return strings.Join(lines, "\n")
}

// pathExists checks if a file or directory exists
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
