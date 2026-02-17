package audit

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	maxSize = 64 * 1024 * 1024 // 64MB
	trimPercent = 25            // Remove 25% when limit reached
)

// Log maintains a centralized audit log of all messages
type Log struct {
	entries    []string
	totalBytes int
	mu         sync.RWMutex
	notifyCh   chan struct{}
}

// NewLog creates a new audit log
func NewLog() *Log {
	return &Log{
		entries:  make([]string, 0, 1000),
		notifyCh: make(chan struct{}, 1),
	}
}

// Append adds a new entry to the audit log
// Format: {timestamp} {message-type} {sender}->{recipient} {subject}: {body}
func (l *Log) Append(msgType, sender, recipient, subject, body string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Format timestamp
	timestamp := time.Now().Format(time.RFC3339)

	// Escape newlines in subject and body for single-line grep/awk
	// But for human readability, we'll use multi-line format with blank line separator
	entry := fmt.Sprintf("%s %s %s->%s\nSubject: %s\n%s\n",
		timestamp, msgType, sender, recipient, subject, body)

	l.entries = append(l.entries, entry)
	l.totalBytes += len(entry)

	// Trim if needed
	if l.totalBytes > maxSize {
		l.trim()
	}

	// Notify waiters
	select {
	case l.notifyCh <- struct{}{}:
	default:
	}
}

// trim removes the oldest 25% of entries
func (l *Log) trim() {
	if len(l.entries) == 0 {
		return
	}

	removeCount := len(l.entries) * trimPercent / 100
	if removeCount == 0 {
		removeCount = 1
	}

	// Calculate bytes to remove
	removedBytes := 0
	for i := 0; i < removeCount && i < len(l.entries); i++ {
		removedBytes += len(l.entries[i])
	}

	// Remove entries
	l.entries = l.entries[removeCount:]
	l.totalBytes -= removedBytes
}

// Read returns the entire audit log as a string
func (l *Log) Read() string {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return strings.Join(l.entries, "\n")
}

// ReadFrom returns audit log content from a specific byte offset
func (l *Log) ReadFrom(offset int64) (string, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	fullLog := strings.Join(l.entries, "\n")
	if offset >= int64(len(fullLog)) {
		return "", false
	}
	return fullLog[offset:], true
}

// WaitForData returns a channel that signals when new data is available
func (l *Log) WaitForData() <-chan struct{} {
	return l.notifyCh
}

// Size returns the current size in bytes
func (l *Log) Size() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.totalBytes
}
