package sandbox

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config represents the global sandbox configuration
type Config struct {
	General    GeneralConfig    `yaml:"general"`
	Filesystem FilesystemConfig `yaml:"filesystem"`
	Network    NetworkConfig    `yaml:"network"`
	Advanced   AdvancedConfig   `yaml:"advanced"`
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
		Advanced: AdvancedConfig{
			LDD:     false, // Disabled: fails on Node.js scripts like claude
			AddExec: true,
		},
	}
}

// ConfigPath returns the path to the config file
func ConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "anvillm", "sandbox.yaml")
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

// expandPath replaces template variables with actual values
func expandPath(pattern, cwd, home, tmpdir string) string {
	s := pattern
	s = strings.ReplaceAll(s, "{CWD}", cwd)
	s = strings.ReplaceAll(s, "{HOME}", home)
	s = strings.ReplaceAll(s, "{TMPDIR}", tmpdir)
	return s
}
