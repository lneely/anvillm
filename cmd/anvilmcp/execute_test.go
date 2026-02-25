package main

import (
	"strings"
	"testing"
)

func TestValidateCode(t *testing.T) {
	tests := []struct {
		name    string
		code    string
		wantErr bool
	}{
		{"safe code", "echo hello", false},
		{"rm -rf /", "rm -rf /", true},
		{"fork bomb", ":(){ :|:& };:", true},
		{"mkfs", "mkfs.ext4 /dev/sda", true},
		{"dd zero", "dd if=/dev/zero of=/dev/sda", true},
		{"chmod 777", "chmod 777 /etc/passwd", true},
		{"curl", "curl http://example.com", true},
		{"wget", "wget http://example.com/file", true},
		{"device write", "echo data > /dev/sda", true},
		{"exec", "exec(malicious)", true},
		{"eval", "eval(dangerous)", true},
		{"safe 9p", "9p read agent/inbox/user", false},
		{"safe jq", "echo '{}' | jq .field", false},
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

func TestLimitedWriter(t *testing.T) {
	tests := []struct {
		name      string
		limit     int
		writes    []string
		wantTotal int
		wantErr   bool
	}{
		{
			name:      "under limit",
			limit:     100,
			writes:    []string{"hello", " ", "world"},
			wantTotal: 11,
			wantErr:   false,
		},
		{
			name:      "at limit",
			limit:     5,
			writes:    []string{"hello"},
			wantTotal: 5,
			wantErr:   false,
		},
		{
			name:      "over limit",
			limit:     5,
			writes:    []string{"hello", "world"},
			wantTotal: 5,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf strings.Builder
			lw := &limitedWriter{w: &buf, limit: tt.limit}

			var lastErr error
			for _, write := range tt.writes {
				_, err := lw.Write([]byte(write))
				if err != nil {
					lastErr = err
					break
				}
			}

			if (lastErr != nil) != tt.wantErr {
				t.Errorf("limitedWriter error = %v, wantErr %v", lastErr, tt.wantErr)
			}
			if buf.Len() != tt.wantTotal {
				t.Errorf("limitedWriter wrote %d bytes, want %d", buf.Len(), tt.wantTotal)
			}
		})
	}
}
