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

	"anvillm/internal/sandbox"
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

func isPermissionError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "permission denied") ||
		strings.Contains(msg, "no such file or directory")
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

// loadLayeredConfig loads sandbox config using the layered approach:
// global.yaml -> backend (anvilmcp) -> sandbox/<name>.yaml
func loadLayeredConfig(name string) (*sandbox.Config, error) {
	// Load global.yaml as base
	baseCfg, err := sandbox.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load global config: %w", err)
	}

	// Convert base config to layered format
	baseLayer := sandbox.LayeredConfig{
		Filesystem: baseCfg.Filesystem,
		Network:    baseCfg.Network,
		Env:        baseCfg.Env,
	}
	layers := []sandbox.LayeredConfig{baseLayer}

	// Load sandbox layer
	if name == "" {
		name = "anvilmcp"
	}
	sbxLayer, err := sandbox.LoadSandbox(name)
	if err != nil {
		return nil, fmt.Errorf("failed to load sandbox %q: %w", name, err)
	}
	layers = append(layers, sbxLayer)

	// Merge layers
	general := sandbox.GeneralConfig{
		BestEffort: baseCfg.General.BestEffort,
		LogLevel:   baseCfg.General.LogLevel,
	}
	advanced := sandbox.AdvancedConfig{
		LDD:     baseCfg.Advanced.LDD,
		AddExec: baseCfg.Advanced.AddExec,
	}

	return sandbox.Merge(general, advanced, layers...), nil
}

func executeCode(code, language string, timeout int, sandboxName string) (string, error) {
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

	// Load layered sandbox config
	cfg, err := loadLayeredConfig(sandboxName)
	if err != nil {
		return "", err
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home dir: %v", err)
	}

	// Use current working directory
	workDir, _ := os.Getwd()

	// For anvilmcp sandbox, use temp workspace
	var cleanupWorkDir bool
	if sandboxName == "" || sandboxName == "anvilmcp" {
		workspaceBase := filepath.Join(homeDir, ".cache", "anvillm", "exec")
		if err := os.MkdirAll(workspaceBase, 0700); err != nil {
			return "", fmt.Errorf("failed to create workspace base: %v", err)
		}
		workDir, err = os.MkdirTemp(workspaceBase, "anvilmcp-*")
		if err != nil {
			return "", fmt.Errorf("failed to create workspace: %v", err)
		}
		cleanupWorkDir = true
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
				expanded := sandbox.ExpandPath(p, workDir)
				if expanded == "" || strings.Contains(expanded, "{") {
					continue // Skip unexpanded templates
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
