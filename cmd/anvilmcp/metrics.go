package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type ExecutionLog struct {
	Timestamp  time.Time     `json:"timestamp"`
	CodeHash   string        `json:"code_hash"`
	Language   string        `json:"language"`
	Duration   time.Duration `json:"duration"`
	Success    bool          `json:"success"`
	OutputSize int           `json:"output_size"`
	Error      string        `json:"error,omitempty"`
}

type TokenLog struct {
	Timestamp      time.Time `json:"timestamp"`
	Method         string    `json:"method"`
	DirectTokens   int       `json:"direct_tokens"`
	CodeExecTokens int       `json:"code_exec_tokens"`
	Reduction      float64   `json:"reduction_percent"`
}

type SecurityEvent struct {
	Timestamp time.Time `json:"timestamp"`
	EventType string    `json:"event_type"`
	Language  string    `json:"language"`
	Details   string    `json:"details"`
}

func hashCode(code string) string {
	h := sha256.Sum256([]byte(code))
	return hex.EncodeToString(h[:])
}

func logExecution(log ExecutionLog) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	
	logDir := filepath.Join(home, ".local", "state", "anvilmcp")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return err
	}
	
	logFile := filepath.Join(logDir, "executions.jsonl")
	// 0644: world-readable, contains only hashes and metadata (no sensitive data)
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	
	data, err := json.Marshal(log)
	if err != nil {
		return err
	}
	
	_, err = fmt.Fprintf(f, "%s\n", data)
	return err
}

func logTokens(log TokenLog) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	
	logDir := filepath.Join(home, ".local", "state", "anvilmcp")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return err
	}
	
	logFile := filepath.Join(logDir, "tokens.jsonl")
	// 0644: world-readable, contains only aggregate metrics (no sensitive data)
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	
	data, err := json.Marshal(log)
	if err != nil {
		return err
	}
	
	_, err = fmt.Fprintf(f, "%s\n", data)
	return err
}

func logSecurityEvent(event SecurityEvent) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	
	logDir := filepath.Join(home, ".local", "state", "anvilmcp")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return err
	}
	
	logFile := filepath.Join(logDir, "security.jsonl")
	// 0600: owner-only, contains security events (rate limits, validation failures, timeouts)
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	
	_, err = fmt.Fprintf(f, "%s\n", data)
	return err
}

func estimateTokens(text string) int {
	return len(text) / 4
}
