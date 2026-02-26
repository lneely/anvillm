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
	"time"
)

var dangerousPatterns = []*regexp.Regexp{
	regexp.MustCompile(`rm\s+-[a-z]*r[a-z]*f[a-z]*\s+/`),  // rm -rf / variants
	regexp.MustCompile(`rm\s+-[a-z]*f[a-z]*r[a-z]*\s+/`),  // rm -fr / variants
	regexp.MustCompile(`:\s*\(\s*\)\s*\{\s*:\s*\|\s*:\s*&`), // fork bomb
	regexp.MustCompile(`\bmkfs\b`),                         // filesystem format
	regexp.MustCompile(`\bdd\b.*\bif=/dev/`),               // dd from device
	regexp.MustCompile(`\bchmod\s+777\b`),                  // world-writable
	regexp.MustCompile(`\b(curl|wget)\s+https?://`),        // network fetch
	regexp.MustCompile(`>\s*/dev/`),                        // write to device
	regexp.MustCompile(`\beval\s*\(`),                      // eval execution
	regexp.MustCompile(`\bexec\s*\(`),                      // exec execution
	regexp.MustCompile(`\b(sudo|su)\b`),                    // privilege escalation
	regexp.MustCompile(`/etc/(passwd|shadow|sudoers)`),     // sensitive files
}

var whitespacePattern = regexp.MustCompile(`\s+`)

func validateCode(code string) error {
	// Normalize: collapse whitespace, lowercase for pattern matching
	normalized := strings.ToLower(code)
	normalized = whitespacePattern.ReplaceAllString(normalized, " ")

	for _, pattern := range dangerousPatterns {
		if pattern.MatchString(normalized) {
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

func executeCode(code, language string, timeout int) (string, error) {
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

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home dir: %v", err)
	}
	
	workspaceBase := filepath.Join(homeDir, ".cache", "anvillm", "exec")
	if err := os.MkdirAll(workspaceBase, 0700); err != nil {
		return "", fmt.Errorf("failed to create workspace base: %v", err)
	}
	
	workDir, err := os.MkdirTemp(workspaceBase, "anvilmcp-*")
	if err != nil {
		return "", fmt.Errorf("failed to create workspace: %v", err)
	}
	defer os.RemoveAll(workDir)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	// Check for landrun
	landrunPath, err := exec.LookPath("landrun")
	if err != nil {
		return "", fmt.Errorf("landrun not found: %v", err)
	}

	// Get namespace for 9p access
	namespace := os.Getenv("NAMESPACE")
	if namespace == "" {
		namespace = fmt.Sprintf("/tmp/ns.%s.:0", os.Getenv("USER"))
	}

	// Get AGENT_ID if set
	agentID := os.Getenv("AGENT_ID")

	var cmd *exec.Cmd
	switch language {
	case "bash", "":
		args := []string{
			"--rwx", workDir,
			"--rox", "/usr",
			"--rox", "/lib",
			"--rox", "/lib64",
			"--rox", "/bin",
			"--ro", "/etc/alternatives",
			"--ro", "/etc/ld.so.cache",
			"--rw", namespace,
			"--env", "NAMESPACE=" + namespace,
			"--env", "PATH=/usr/bin:/bin",
			"--env", "HOME=" + homeDir,
		}
		if agentID != "" {
			args = append(args, "--env", "AGENT_ID="+agentID)
		}
		args = append(args, "--", "bash", "-c", code)
		
		cmd = exec.CommandContext(ctx, landrunPath, args...)
		cmd.Dir = workDir
	default:
		return "", fmt.Errorf("unsupported language: %s (only bash supported)", language)
	}

	// Limit output size (10MB)
	var outputBuf bytes.Buffer
	limitedWriter := &limitedWriter{w: &outputBuf, limit: 10 * 1024 * 1024}
	cmd.Stdout = limitedWriter
	cmd.Stderr = limitedWriter

	err = cmd.Run()
	output := outputBuf.Bytes()
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
		return string(output), fmt.Errorf("execution failed: %v", err)
	}

	logExecution(execLog)
	return string(output), nil
}

type limitedWriter struct {
	w       io.Writer
	written int
	limit   int
}

func (lw *limitedWriter) Write(p []byte) (n int, err error) {
	if lw.written >= lw.limit {
		return len(p), nil // Discard silently, report all bytes consumed
	}

	remaining := lw.limit - lw.written
	toWrite := p
	if len(p) > remaining {
		toWrite = p[:remaining]
	}

	written, err := lw.w.Write(toWrite)
	lw.written += written
	if err != nil {
		return written, err
	}
	return len(p), nil // Report all input bytes as consumed
}
