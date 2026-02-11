package sandbox

import (
	"os/exec"
	"strings"
	"sync"
)

var (
	available     bool
	availableOnce sync.Once
)

// IsAvailable checks if landrun is available on the system.
// The result is cached after the first call for performance.
func IsAvailable() bool {
	availableOnce.Do(func() {
		_, err := exec.LookPath("landrun")
		available = (err == nil)
	})
	return available
}

// Version returns the landrun version string, or empty if unavailable.
func Version() string {
	if !IsAvailable() {
		return ""
	}

	cmd := exec.Command("landrun", "--version")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(out))
}
