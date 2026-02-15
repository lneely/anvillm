package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config represents the global sandbox configuration
type Config struct {
	General    GeneralConfig    `yaml:"general"`
	Filesystem FilesystemConfig `yaml:"filesystem"`
	Network    NetworkConfig    `yaml:"network"`
	Env        []string         `yaml:"env"`
	Advanced   AdvancedConfig   `yaml:"advanced"`
}

// LayeredConfig represents a single layer (system/backend/role/task)
type LayeredConfig struct {
	Filesystem FilesystemConfig `yaml:"filesystem"`
	Network    NetworkConfig    `yaml:"network"`
	Env        []string         `yaml:"env"`
}

// GeneralConfig contains general sandbox settings
type GeneralConfig struct {
	BestEffort bool   `yaml:"best_effort"`
	LogLevel   string `yaml:"log_level"`
}

// FilesystemConfig contains filesystem permission settings
type FilesystemConfig struct {
	RO  []string `yaml:"ro"`  // Read-only
	ROX []string `yaml:"rox"` // Read-only with execute
	RW  []string `yaml:"rw"`  // Read-write
	RWX []string `yaml:"rwx"` // Read-write with execute
}

// NetworkConfig contains network permission settings
type NetworkConfig struct {
	Enabled      bool     `yaml:"enabled"`
	Unrestricted bool     `yaml:"unrestricted"`
	BindTCP      []string `yaml:"bind_tcp"`
	ConnectTCP   []string `yaml:"connect_tcp"`
}

// AdvancedConfig contains advanced landrun settings
type AdvancedConfig struct {
	LDD     bool `yaml:"ldd"`
	AddExec bool `yaml:"add_exec"`
}

// DefaultConfig returns the default locked-down configuration
func DefaultConfig() *Config {
	return &Config{
		General: GeneralConfig{
			BestEffort: false, // Fail-closed: refuse to run if sandboxing unavailable
			LogLevel:   "error",
		},
		Filesystem: FilesystemConfig{
			RO: []string{
				"/etc/passwd",       // UID→homedir lookup (getpwuid)
				"/dev/null",         // Null device
				"/proc/meminfo",     // Memory info
				"/proc/self/cgroup", // Cgroup info
				"/proc/self/maps",   // Process memory maps
				"/proc/version",     // Kernel version
			},
			ROX: []string{
				"/usr",
				"/lib",
				"/lib64",
				"/bin",
				"/sbin",
			},
			RW: []string{
				"{CWD}",
				"{TMPDIR}",
				"{HOME}/.claude",
				"{HOME}/.kiro",
				"{HOME}/.config/anvillm",
				"{HOME}/.claude.json",
			},
			RWX: []string{},
		},
		Network: NetworkConfig{
			Enabled:      false,
			Unrestricted: true, // Unrestricted network (fine-grained restrictions require Landlock v5+)
			BindTCP:      []string{},
			ConnectTCP:   []string{"443"}, // Example: HTTPS for API calls (when Enabled: true)
		},
		Env: []string{
			"HOME",
			"USER",
			"PATH",
			"LANG",
			"TERM",
		},
		Advanced: AdvancedConfig{
			LDD:     false, // Disabled: fails on Node.js scripts like claude
			AddExec: true,
		},
	}
}

// ConfigPath returns the path to the config file
func ConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "anvillm", "global.yaml")
}

// Load loads the config from disk, creating defaults if it doesn't exist
func Load() (*Config, error) {
	path := ConfigPath()

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		// Create default config
		cfg := DefaultConfig()
		if saveErr := Save(cfg); saveErr != nil {
			// Return defaults even if save fails
			return cfg, saveErr
		}
		return cfg, nil
	}
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// Save saves the config to disk with explanatory comments
func Save(cfg *Config) error {
	path := ConfigPath()

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	// Prepend header with documentation
	header := `# AnviLLM Sandbox Configuration
# This file controls landrun sandboxing for all backend sessions
# Documentation: https://github.com/landlock-lsm/landrun
#
# Path templates:
#   {CWD}     - Session working directory
#   {HOME}    - User home directory
#   {TMPDIR}  - Temporary directory (/tmp or $TMPDIR)
#
# WARNING: CLI tools need network access to function!
#   - Claude requires connect_tcp for claude.com
#   - Kiro requires connect_tcp for kiro.ai
#
# Changes apply to NEW sessions only. Existing sessions are not affected.

`
	data = append([]byte(header), data...)

	return os.WriteFile(path, data, 0644)
}

// DefaultRole is the role used when none is specified
const DefaultRole = "default"

// validateName checks if a role/task name is safe for use in file paths
func validateName(name string) error {
	if name == "" {
		return fmt.Errorf("name cannot be empty")
	}
	if len(name) > 64 {
		return fmt.Errorf("name too long (max 64 characters)")
	}
	// Only allow alphanumeric, hyphen, and underscore
	for _, r := range name {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
			return fmt.Errorf("name contains invalid character: %c (only alphanumeric, hyphen, underscore allowed)", r)
		}
	}
	return nil
}

// expandPath replaces template variables with actual values
func expandPath(pattern, cwd string) string {
	s := pattern

	// CWD is special - passed as parameter, not from env
	s = strings.ReplaceAll(s, "{CWD}", cwd)

	// Generic environment variable expansion: {VAR_NAME}
	re := regexp.MustCompile(`\{([A-Z_][A-Z0-9_]*)\}`)
	s = re.ReplaceAllStringFunc(s, func(match string) string {
		varName := match[1 : len(match)-1] // Strip { and }

		// Special handling with defaults
		home, _ := os.UserHomeDir()
		switch varName {
		case "HOME":
			if home != "" {
				return home
			}
		case "TMPDIR":
			if tmpdir := os.Getenv("TMPDIR"); tmpdir != "" {
				return tmpdir
			}
			return "/tmp"
		case "XDG_CONFIG_HOME":
			if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
				return xdg
			}
			return filepath.Join(home, ".config")
		case "XDG_DATA_HOME":
			if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
				return xdg
			}
			return filepath.Join(home, ".local/share")
		case "XDG_CACHE_HOME":
			if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
				return xdg
			}
			return filepath.Join(home, ".cache")
		case "XDG_STATE_HOME":
			if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
				return xdg
			}
			return filepath.Join(home, ".local/state")
		case "XDG_RUNTIME_DIR":
			if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
				return xdg
			}
			return fmt.Sprintf("/run/user/%d", os.Getuid())
		case "XDG_DOCUMENTS_DIR":
			if xdg := os.Getenv("XDG_DOCUMENTS_DIR"); xdg != "" {
				return xdg
			}
			return filepath.Join(home, "Documents")
		case "XDG_DOWNLOAD_DIR":
			if xdg := os.Getenv("XDG_DOWNLOAD_DIR"); xdg != "" {
				return xdg
			}
			return filepath.Join(home, "Downloads")
		case "XDG_MUSIC_DIR":
			if xdg := os.Getenv("XDG_MUSIC_DIR"); xdg != "" {
				return xdg
			}
			return filepath.Join(home, "Music")
		case "XDG_PICTURES_DIR":
			if xdg := os.Getenv("XDG_PICTURES_DIR"); xdg != "" {
				return xdg
			}
			return filepath.Join(home, "Pictures")
		case "XDG_VIDEOS_DIR":
			if xdg := os.Getenv("XDG_VIDEOS_DIR"); xdg != "" {
				return xdg
			}
			return filepath.Join(home, "Videos")
		}

		// Generic env var lookup
		if val := os.Getenv(varName); val != "" {
			return val
		}
		return match // Keep original if env var not set
	})

	return s
}

// SystemDefaults returns the system-level defaults (restrictive baseline)
func SystemDefaults() LayeredConfig {
	return LayeredConfig{
		Filesystem: FilesystemConfig{
			RO: []string{
				"/etc/passwd",
				"/dev/null",
				"/proc/meminfo",
				"/proc/self/cgroup",
				"/proc/self/maps",
				"/proc/version",
			},
			ROX: []string{
				"/usr",
				"/lib",
				"/lib64",
				"/bin",
				"/sbin",
			},
			RW: []string{
				"{TMPDIR}",
			},
		},
		Network: NetworkConfig{
			Enabled:      false,
			Unrestricted: false,
		},
		Env: []string{
			"HOME",
			"USER",
			"PATH",
			"LANG",
			"TERM",
		},
	}
}

// LoadBackend loads backend-specific config
func LoadBackend(name string) (LayeredConfig, error) {
	if err := validateName(name); err != nil {
		return LayeredConfig{}, fmt.Errorf("invalid backend name: %w", err)
	}

	home, _ := os.UserHomeDir()
	path := filepath.Join(home, ".config", "anvillm", "backends", name+".yaml")

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return LayeredConfig{}, fmt.Errorf("backend %q not found (looked in %s)", name, path)
	}
	if err != nil {
		return LayeredConfig{}, err
	}

	var cfg LayeredConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return LayeredConfig{}, fmt.Errorf("parse %s: %w", path, err)
	}

	return cfg, nil
}

// LoadRole loads role-specific config
func LoadRole(name string) (LayeredConfig, error) {
	if err := validateName(name); err != nil {
		return LayeredConfig{}, fmt.Errorf("invalid role name: %w", err)
	}

	home, _ := os.UserHomeDir()
	path := filepath.Join(home, ".config", "anvillm", "roles", name+".yaml")

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return LayeredConfig{}, fmt.Errorf("role %q not found (looked in %s)", name, path)
	}
	if err != nil {
		return LayeredConfig{}, err
	}

	var cfg LayeredConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return LayeredConfig{}, fmt.Errorf("parse %s: %w", path, err)
	}

	return cfg, nil
}

// LoadTask loads task-specific config
func LoadTask(name string) (LayeredConfig, error) {
	if err := validateName(name); err != nil {
		return LayeredConfig{}, fmt.Errorf("invalid task name: %w", err)
	}

	home, _ := os.UserHomeDir()
	path := filepath.Join(home, ".config", "anvillm", "tasks", name+".yaml")

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return LayeredConfig{}, fmt.Errorf("task %q not found (looked in %s)", name, path)
	}
	if err != nil {
		return LayeredConfig{}, err
	}

	var cfg LayeredConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return LayeredConfig{}, fmt.Errorf("parse %s: %w", path, err)
	}

	return cfg, nil
}

// Merge combines multiple layers into a final Config (most permissive wins)
func Merge(general GeneralConfig, advanced AdvancedConfig, layers ...LayeredConfig) *Config {
	cfg := &Config{
		General:  general,
		Advanced: advanced,
		Filesystem: FilesystemConfig{
			RO:  []string{},
			ROX: []string{},
			RW:  []string{},
			RWX: []string{},
		},
		Network: NetworkConfig{
			Enabled:      false,
			Unrestricted: false,
		},
		Env: []string{},
	}

	// Track paths by permission level (higher = more permissive)
	pathPerms := make(map[string]int) // 0=none, 1=ro, 2=rox, 3=rw, 4=rwx
	envSet := make(map[string]bool)

	for _, layer := range layers {
		// Merge filesystem permissions (most permissive wins)
		for _, p := range layer.Filesystem.RO {
			if pathPerms[p] < 1 {
				pathPerms[p] = 1
			}
		}
		for _, p := range layer.Filesystem.ROX {
			if pathPerms[p] < 2 {
				pathPerms[p] = 2
			}
		}
		for _, p := range layer.Filesystem.RW {
			if pathPerms[p] < 3 {
				pathPerms[p] = 3
			}
		}
		for _, p := range layer.Filesystem.RWX {
			if pathPerms[p] < 4 {
				pathPerms[p] = 4
			}
		}

		// Merge network (most permissive wins)
		if layer.Network.Enabled {
			cfg.Network.Enabled = true
		}
		if layer.Network.Unrestricted {
			cfg.Network.Unrestricted = true
		}
		cfg.Network.BindTCP = append(cfg.Network.BindTCP, layer.Network.BindTCP...)
		cfg.Network.ConnectTCP = append(cfg.Network.ConnectTCP, layer.Network.ConnectTCP...)

		// Merge environment (union)
		for _, e := range layer.Env {
			envSet[e] = true
		}
	}

	// Convert path permissions back to lists
	for path, perm := range pathPerms {
		switch perm {
		case 1:
			cfg.Filesystem.RO = append(cfg.Filesystem.RO, path)
		case 2:
			cfg.Filesystem.ROX = append(cfg.Filesystem.ROX, path)
		case 3:
			cfg.Filesystem.RW = append(cfg.Filesystem.RW, path)
		case 4:
			cfg.Filesystem.RWX = append(cfg.Filesystem.RWX, path)
		}
	}

	// Convert env set to list
	for e := range envSet {
		cfg.Env = append(cfg.Env, e)
	}

	return cfg
}
