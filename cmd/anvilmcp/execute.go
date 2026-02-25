package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func validateCode(code string) error {
	dangerous := []string{
		"rm -rf /",
		":(){ :|:& };:",
		"mkfs",
		"dd if=/dev/zero",
		"chmod 777",
		"curl http",
		"wget http",
		"> /dev/",
		"exec(",
		"eval(",
	}
	for _, pattern := range dangerous {
		if strings.Contains(code, pattern) {
			logSecurityEvent(SecurityEvent{
				Timestamp: time.Now(),
				EventType: "validation_failure",
				Details:   fmt.Sprintf("dangerous pattern: %s", pattern),
			})
			return fmt.Errorf("dangerous pattern detected: %s", pattern)
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
		return 0, fmt.Errorf("output size limit exceeded (%d bytes)", lw.limit)
	}
	
	remaining := lw.limit - lw.written
	if len(p) > remaining {
		p = p[:remaining]
	}
	
	n, err = lw.w.Write(p)
	lw.written += n
	return n, err
}
