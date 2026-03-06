package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

var dangerousPatterns = []*regexp.Regexp{
	regexp.MustCompile(`rm\s+(-[a-z]*r[a-z]*\s+)*-[a-z]*f[a-z]*\s*/(home|var|usr|etc|boot|root|bin|sbin|lib|opt|srv)?`), // rm -rf, rm -r -f on sensitive paths
	regexp.MustCompile(`rm\s+(-[a-z]*f[a-z]*\s+)*-[a-z]*r[a-z]*\s*/(home|var|usr|etc|boot|root|bin|sbin|lib|opt|srv)?`), // rm -fr, rm -f -r on sensitive paths
	regexp.MustCompile(`rm\s+.*--recursive.*--force`),                                                                    // rm --recursive --force
	regexp.MustCompile(`rm\s+.*--force.*--recursive`),                                                                    // rm --force --recursive
	regexp.MustCompile(`rm\s+(-[a-z]*r[a-z]*\s+)*-[a-z]*f[a-z]*\s+\.\.?(/|$)`),                                           // rm -rf ./ or rm -rf ../
	regexp.MustCompile(`rm\s+(-[a-z]*r[a-z]*\s+)*-[a-z]*f[a-z]*\s+~`),                                                    // rm -rf ~ (home dir)
	regexp.MustCompile(`rm\s+(-[a-z]*r[a-z]*\s+)*-[a-z]*f[a-z]*\s+\*`),                                                   // rm -rf * (glob expansion)
	regexp.MustCompile(`:\s*\(\s*\)\s*\{\s*:\s*\|\s*:\s*&`),                                                              // fork bomb
	regexp.MustCompile(`\bmkfs\b`),                                                                                       // filesystem format
	regexp.MustCompile(`\bdd\b.*\bif=/dev/`),                                                                             // dd from device
	regexp.MustCompile(`>\s*/dev/sd`),                                                                                    // write to block device
	regexp.MustCompile(`\beval\s+".*\$`),                                                                                 // eval with variable expansion
	regexp.MustCompile(`\b(sudo|su)\s`),                                                                                  // privilege escalation
	regexp.MustCompile(`/etc/(shadow|sudoers)`),                                                                          // sensitive files (not passwd)
}

var whitespacePattern = regexp.MustCompile(`\s+`)

// Rate limiting for validation failures
var (
	rateLimitMu       sync.Mutex
	validationFailures int
	lastFailure       time.Time
	blockedUntil      time.Time
)

const (
	maxFailures     = 5
	blockDuration   = 30 * time.Second
	failureWindow   = 60 * time.Second
)

func checkRateLimit() error {
	rateLimitMu.Lock()
	defer rateLimitMu.Unlock()

	now := time.Now()
	if now.Before(blockedUntil) {
		remaining := blockedUntil.Sub(now).Round(time.Second)
		return fmt.Errorf("rate limited: too many validation failures, blocked for %v", remaining)
	}
	return nil
}

func recordValidationFailure() {
	rateLimitMu.Lock()
	defer rateLimitMu.Unlock()

	now := time.Now()
	// Reset counter if outside failure window
	if now.Sub(lastFailure) > failureWindow {
		validationFailures = 0
	}
	
	validationFailures++
	lastFailure = now
	
	if validationFailures >= maxFailures {
		blockedUntil = now.Add(blockDuration)
		validationFailures = 0
		logSecurityEvent(SecurityEvent{
			Timestamp: now,
			EventType: "rate_limit_triggered",
			Details:   fmt.Sprintf("blocked for %v after %d failures", blockDuration, maxFailures),
		})
	}
}

func validateCode(code string) error {
	if err := checkRateLimit(); err != nil {
		return err
	}

	// Normalize: collapse whitespace, lowercase for pattern matching
	normalized := strings.ToLower(code)
	normalized = whitespacePattern.ReplaceAllString(normalized, " ")

	for _, pattern := range dangerousPatterns {
		if pattern.MatchString(normalized) {
			recordValidationFailure()
			logSecurityEvent(SecurityEvent{
				Timestamp: time.Now(),
				EventType: "validation_failure",
				Details:   fmt.Sprintf("dangerous pattern: %s", pattern.String()),
			})
			return fmt.Errorf("dangerous pattern detected")
		}
	}
	return nil
}

// SandboxConfig represents the sandbox configuration loaded from YAML
type SandboxConfig struct {
	CWD        string `yaml:"cwd"` // "temp" for temp workspace, or path template like "{CWD}"
	Filesystem struct {
		RO  []string `yaml:"ro"`
		ROX []string `yaml:"rox"`
		RW  []string `yaml:"rw"`
		RWX []string `yaml:"rwx"`
	} `yaml:"filesystem"`
	Env     []string `yaml:"env"`
	Network struct {
		Unrestricted bool `yaml:"unrestricted"`
	} `yaml:"network"`
}

// loadSandboxConfig loads sandbox config from ~/.config/anvillm/sandbox/<name>.yaml
// Falls back to ./cfg/sandbox/<name>.yaml for development
func loadSandboxConfig(name string) (*SandboxConfig, error) {
	// Validate name to prevent path traversal
	if name == "" {
		name = "anvilmcp"
	}
	for _, r := range name {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
			return nil, fmt.Errorf("invalid sandbox name: %s", name)
		}
	}

	homeDir, _ := os.UserHomeDir()
	cfgPath := filepath.Join(homeDir, ".config", "anvillm", "sandbox", name+".yaml")

	// Fall back to ./cfg/sandbox/ for development
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		cfgPath = filepath.Join("cfg", "sandbox", name+".yaml")
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load sandbox config %s: %v", name, err)
	}

	var cfg SandboxConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse sandbox config: %v", err)
	}

	return &cfg, nil
}

// expandPath replaces template variables with actual values
func expandPath(pattern string) string {
	s := pattern
	homeDir, _ := os.UserHomeDir()

	// Generic environment variable expansion: {VAR_NAME}
	re := regexp.MustCompile(`\{([A-Z_][A-Z0-9_]*)\}`)
	s = re.ReplaceAllStringFunc(s, func(match string) string {
		varName := match[1 : len(match)-1]

		switch varName {
		case "HOME":
			return homeDir
		case "PLAN9":
			if plan9 := os.Getenv("PLAN9"); plan9 != "" {
				return plan9
			}
			return "" // Will cause path to be skipped
		case "NAMESPACE":
			if ns := os.Getenv("NAMESPACE"); ns != "" {
				return ns
			}
			if nsCmd, err := exec.Command("namespace").Output(); err == nil {
				return strings.TrimSpace(string(nsCmd))
			}
			return fmt.Sprintf("/tmp/ns.%s.:0", os.Getenv("USER"))
		case "CWD":
			return "" // Handled specially - replaced with workDir
		}

		if val := os.Getenv(varName); val != "" {
			return val
		}
		return "" // Will cause path to be skipped
	})

	return s
}

func executeCode(code, language string, timeout int, sandbox string) (string, error) {
	start := time.Now()
	
	if timeout <= 0 {
		timeout = 30
	}

	if err := validateCode(code); err != nil {
		logExecution(ExecutionLog{
			Timestamp:  start,
			CodeHash:   hashCode(code),
			Language:   language,
			Duration:   time.Since(start),
			Success:    false,
			OutputSize: 0,
			Error:      err.Error(),
		})
		return "", err
	}

	// Load sandbox config
	cfg, err := loadSandboxConfig(sandbox)
	if err != nil {
		return "", err
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home dir: %v", err)
	}

	// Determine working directory based on config
	var workDir string
	var cleanupWorkDir bool
	
	if cfg.CWD == "" || cfg.CWD == "temp" {
		// Use temp workspace (original behavior)
		workspaceBase := filepath.Join(homeDir, ".cache", "anvillm", "exec")
		if err := os.MkdirAll(workspaceBase, 0700); err != nil {
			return "", fmt.Errorf("failed to create workspace base: %v", err)
		}
		workDir, err = os.MkdirTemp(workspaceBase, "anvilmcp-*")
		if err != nil {
			return "", fmt.Errorf("failed to create workspace: %v", err)
		}
		cleanupWorkDir = true
	} else {
		// Use configured path (e.g., "{CWD}" expands to current directory)
		workDir = expandPath(cfg.CWD)
		if workDir == "" {
			workDir, _ = os.Getwd()
		}
		cleanupWorkDir = false
	}
	
	if cleanupWorkDir {
		defer os.RemoveAll(workDir)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	landrunPath, err := exec.LookPath("landrun")
	if err != nil {
		return "", fmt.Errorf("landrun not found: %v", err)
	}

	var cmd *exec.Cmd
	switch language {
	case "bash", "":
		args := []string{}

		// Add filesystem permissions from config
		addPaths := func(flag string, paths []string) {
			for _, p := range paths {
				expanded := expandPath(p)
				if expanded == "" {
					continue
				}
				// Handle {CWD} specially
				if strings.Contains(p, "{CWD}") {
					expanded = strings.ReplaceAll(p, "{CWD}", workDir)
				}
				if _, err := os.Stat(expanded); err == nil {
					args = append(args, flag, expanded)
				}
			}
		}

		addPaths("--ro", cfg.Filesystem.RO)
		addPaths("--rox", cfg.Filesystem.ROX)
		addPaths("--rw", cfg.Filesystem.RW)
		addPaths("--rwx", cfg.Filesystem.RWX)

		// Network
		if cfg.Network.Unrestricted {
			args = append(args, "--unrestricted-network")
		}

		// Environment variables
		for _, env := range cfg.Env {
			if os.Getenv(env) != "" || env == "HOME" || env == "USER" || env == "PATH" {
				args = append(args, "--env", env)
			}
		}

		args = append(args, "--", "bash", "-c", code)
		
		cmd = exec.CommandContext(ctx, landrunPath, args...)
		cmd.Dir = workDir
	default:
		return "", fmt.Errorf("unsupported language: %s (only bash supported)", language)
	}

	// Limit output size (10MB)
	var outputBuf bytes.Buffer
	lw := &limitedWriter{w: &outputBuf, limit: 10 * 1024 * 1024}
	cmd.Stdout = lw
	cmd.Stderr = lw

	err = cmd.Run()
	output := outputBuf.Bytes()
	
	if lw.truncated {
		output = append(output, []byte("\n[output truncated at 10MB]")...)
	}
	
	duration := time.Since(start)
	
	execLog := ExecutionLog{
		Timestamp:  start,
		CodeHash:   hashCode(code),
		Language:   language,
		Duration:   duration,
		Success:    err == nil && ctx.Err() != context.DeadlineExceeded,
		OutputSize: len(output),
	}
	
	if ctx.Err() == context.DeadlineExceeded {
		execLog.Error = fmt.Sprintf("execution timeout after %d seconds", timeout)
		logExecution(execLog)
		logSecurityEvent(SecurityEvent{
			Timestamp: start,
			EventType: "timeout",
			Language:  language,
			Details:   fmt.Sprintf("timeout after %d seconds", timeout),
		})
		return "", fmt.Errorf("execution timeout after %d seconds", timeout)
	}
	if err != nil {
		execLog.Error = err.Error()
		logExecution(execLog)
		return string(output), fmt.Errorf("execution failed: %v\nOutput: %s", err, string(output))
	}

	logExecution(execLog)
	return string(output), nil
}

type limitedWriter struct {
	w         io.Writer
	written   int
	limit     int
	truncated bool
}

func (lw *limitedWriter) Write(p []byte) (n int, err error) {
	if lw.written >= lw.limit {
		lw.truncated = true
		return len(p), nil // Discard, report all bytes consumed
	}

	remaining := lw.limit - lw.written
	toWrite := p
	if len(p) > remaining {
		toWrite = p[:remaining]
		lw.truncated = true
	}

	written, err := lw.w.Write(toWrite)
	lw.written += written
	if err != nil {
		return written, err
	}
	return len(p), nil // Report all input bytes as consumed
}
