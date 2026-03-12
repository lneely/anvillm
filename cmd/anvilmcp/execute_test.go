package main

import (
	"strings"
	"testing"
	"time"
)

func TestValidateCode(t *testing.T) {
	tests := []struct {
		name    string
		code    string
		wantErr bool
	}{
		{"safe code", "echo hello", false},
		{"rm -rf /", "rm -rf /", true},
		{"rm -rf / with spaces", "rm  -rf  /", true},
		{"rm -rf / with tabs", "rm\t-rf\t/", true},
		{"rm -rf/ no space", "rm -rf/", true},
		{"rm -fr /", "rm -fr /", true},
		{"rm -fr/ no space", "rm -fr/", true},
		{"rm -rf /home", "rm -rf /home", true},
		{"rm -rf /var", "rm -rf /var", true},
		{"rm -rf /etc", "rm -rf /etc", true},
		{"rm -rf /usr", "rm -rf /usr", true},
		{"fork bomb", ":(){ :|:& };:", true},
		{"fork bomb with spaces", ": ( ) { : | : & } ; :", true},
		{"mkfs", "mkfs.ext4 /dev/sda", true},
		{"dd from device", "dd if=/dev/zero of=/dev/sda", true},
		{"chmod 777", "chmod 777 /etc/passwd", true},
		{"curl http", "curl http://example.com", true},
		{"curl https", "curl https://example.com", true},
		{"curl ftp", "curl ftp://example.com/file", true},
		{"curl file", "curl file:///etc/passwd", true},
		{"wget http", "wget http://example.com/file", true},
		{"wget https", "wget https://example.com/file", true},
		{"wget ftp", "wget ftp://example.com/file", true},
		{"device write", "echo data > /dev/sda", true},
		{"exec", "exec(malicious)", true},
		{"eval", "eval(dangerous)", true},
		{"sudo", "sudo rm file", true},
		{"su", "su - root", true},
		{"etc passwd", "cat /etc/passwd", true},
		{"etc shadow", "cat /etc/shadow", true},
		{"safe 9p", "9p read anvillm/inbox/user", false},
		{"safe jq", "echo '{}' | jq .field", false},
		{"safe rm", "rm file.txt", false},
		{"rm -rf ./", "rm -rf ./", true},
		{"rm -rf ../", "rm -rf ../", true},
		{"rm -rf .", "rm -rf .", true},
		{"rm -r -f /", "rm -r -f /", true},
		{"rm -f -r /", "rm -f -r /", true},
		{"rm --recursive --force /", "rm --recursive --force /", true},
		{"rm --force --recursive /", "rm --force --recursive /", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCode(tt.code)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateCode() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDangerousPatternsCompile(t *testing.T) {
	// Verify all patterns are valid regexes (they compile at init, but this documents the expectation)
	if len(dangerousPatterns) == 0 {
		t.Error("dangerousPatterns should not be empty")
	}
	for i, p := range dangerousPatterns {
		if p == nil {
			t.Errorf("dangerousPatterns[%d] is nil", i)
		}
	}
}

func TestLimitedWriter(t *testing.T) {
	tests := []struct {
		name          string
		limit         int
		writes        []string
		wantTotal     int
		wantTruncated bool
	}{
		{
			name:          "under limit",
			limit:         100,
			writes:        []string{"hello", " ", "world"},
			wantTotal:     11,
			wantTruncated: false,
		},
		{
			name:          "at limit",
			limit:         5,
			writes:        []string{"hello"},
			wantTotal:     5,
			wantTruncated: false,
		},
		{
			name:          "over limit",
			limit:         5,
			writes:        []string{"hello", "world"},
			wantTotal:     5,
			wantTruncated: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf strings.Builder
			lw := &limitedWriter{w: &buf, limit: tt.limit}

			for _, write := range tt.writes {
				lw.Write([]byte(write))
			}

			if lw.truncated != tt.wantTruncated {
				t.Errorf("limitedWriter truncated = %v, want %v", lw.truncated, tt.wantTruncated)
			}
			if buf.Len() != tt.wantTotal {
				t.Errorf("limitedWriter wrote %d bytes, want %d", buf.Len(), tt.wantTotal)
			}
		})
	}
}

func resetRateLimitState() {
	rateLimitMu.Lock()
	defer rateLimitMu.Unlock()
	validationFailures = 0
	lastFailure = time.Time{}
	blockedUntil = time.Time{}
}

func TestRateLimitCounterResetAfterWindow(t *testing.T) {
	resetRateLimitState()
	defer resetRateLimitState()

	// Record failures but not enough to trigger block
	for i := 0; i < maxFailures-1; i++ {
		recordValidationFailure()
	}

	// Simulate time passing beyond failureWindow
	rateLimitMu.Lock()
	lastFailure = time.Now().Add(-failureWindow - time.Second)
	rateLimitMu.Unlock()

	// Next failure should reset counter, not trigger block
	recordValidationFailure()

	if err := checkRateLimit(); err != nil {
		t.Errorf("expected no rate limit after window reset, got: %v", err)
	}
}

func TestRateLimitBlocksAfterMaxFailures(t *testing.T) {
	resetRateLimitState()
	defer resetRateLimitState()

	for i := 0; i < maxFailures; i++ {
		recordValidationFailure()
	}

	if err := checkRateLimit(); err == nil {
		t.Error("expected rate limit error after maxFailures")
	}
}

func TestRateLimitUnblocksAfterDuration(t *testing.T) {
	resetRateLimitState()
	defer resetRateLimitState()

	for i := 0; i < maxFailures; i++ {
		recordValidationFailure()
	}

	// Simulate block duration passing
	rateLimitMu.Lock()
	blockedUntil = time.Now().Add(-time.Second)
	rateLimitMu.Unlock()

	if err := checkRateLimit(); err != nil {
		t.Errorf("expected no rate limit after block duration, got: %v", err)
	}
}
