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

	"9fans.net/go/plan9/client"
	"anvillm/internal/sandbox"
)

// readTool reads a tool from the 9P tools directory.
func readTool(name string) (string, error) {
	// Prevent path traversal
	if strings.Contains(name, "/") || strings.Contains(name, "..") {
		return "", fmt.Errorf("invalid tool name")
	}

	ns := fmt.Sprintf("/tmp/ns.%s.:0", os.Getenv("USER"))
	fsys, err := client.Mount("unix", filepath.Join(ns, "anvillm"))
	if err != nil {
		return "", fmt.Errorf("failed to mount 9P: %v", err)
	}
	defer fsys.Close()

	fid, err := fsys.Open("/tools/"+name, 0)
	if err != nil {
		return "", fmt.Errorf("tool not found: %s", name)
	}
	defer fid.Close()

	var buf []byte
	tmp := make([]byte, 8192)
	for {
		n, err := fid.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil || n < len(tmp) {
			break
		}
	}
	return string(buf), nil
}

// PipeStep is one stage in a tool pipeline.
// Exactly one of Tool or Code must be set.
type PipeStep struct {
	Tool string // named tool read from 9P (trusted)
	Code string // inline bash code (untrusted, validated)
	Args []string
}

// buildPipeline constructs a single bash pipeline string from the given steps.
// Each step is wrapped in a subshell: ( set -- args; <code> ) | ...
// Tool steps are trusted (sourced from 9P); inline code steps are validated
// individually here so the combined string is always returned as trusted.
func buildPipeline(steps []PipeStep) (string, bool, error) {
	if len(steps) == 0 {
		return "", false, fmt.Errorf("pipe requires at least one step")
	}
	parts := make([]string, 0, len(steps))
	for _, step := range steps {
		var code string
		if step.Tool != "" {
			var err error
			code, err = readTool(step.Tool)
			if err != nil {
				return "", false, fmt.Errorf("pipe step %q: %v", step.Tool, err)
			}
		} else if step.Code != "" {
			if err := validateCode(step.Code); err != nil {
				return "", false, fmt.Errorf("pipe step code: %v", err)
			}
			code = step.Code
		} else {
			return "", false, fmt.Errorf("each pipe step requires either 'tool' or 'code'")
		}
		if len(step.Args) > 0 {
			var escaped []string
			for _, arg := range step.Args {
				escaped = append(escaped, "'"+strings.ReplaceAll(arg, "'", "'\\''")+"'")
			}
			parts = append(parts, fmt.Sprintf("( set -- %s\n%s )", strings.Join(escaped, " "), code))
		} else {
			parts = append(parts, fmt.Sprintf("(\n%s\n)", code))
		}
	}
	return strings.Join(parts, " |\n"), true, nil
}

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

func executeCode(code, language string, timeout int, sandboxName string, trusted bool) (string, error) {
	start := time.Now()
	
	if timeout <= 0 {
		timeout = 30
	}

	if !trusted {
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

	var cmd *exec.Cmd
	switch language {
	case "bash", "":
		wrapped := sandbox.WrapCommand(cfg, []string{"bash", "-c", code}, workDir)
		cmd = exec.CommandContext(ctx, wrapped[0], wrapped[1:]...)
		cmd.Dir = workDir
	// Add new language cases here:
	// case "python":
	//     wrapped := sandbox.WrapCommand(cfg, []string{"python3", "-c", code}, workDir)
	default:
		return "", fmt.Errorf("unsupported language: %s (supported: bash)", language)
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
