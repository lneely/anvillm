// Package tmux provides tmux-based backend implementation for CLI tools
package tmux

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

// tmuxCmd executes a tmux command and returns the output
func tmuxCmd(args ...string) (string, error) {
	cmd := exec.Command("tmux", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("tmux %s: %w: %s", strings.Join(args, " "), err, string(out))
	}
	return string(out), nil
}

// createSession creates a new detached tmux session
func createSession(name string, rows, cols uint16) error {
	_, err := tmuxCmd("new-session", "-d", "-s", name,
		"-x", fmt.Sprintf("%d", cols),
		"-y", fmt.Sprintf("%d", rows))
	return err
}

// setEnvironment sets an environment variable in the tmux session
func setEnvironment(session, key, value string) error {
	_, err := tmuxCmd("set-environment", "-t", session, key, value)
	return err
}

// sendKeys sends keys to a tmux session
// Special keys: C-m (Enter), C-c (Ctrl+C), etc.
func sendKeys(session string, keys ...string) error {
	args := []string{"send-keys", "-t", session}
	args = append(args, keys...)
	_, err := tmuxCmd(args...)
	return err
}

// sendLiteral sends literal text to a tmux session (doesn't interpret special chars)
func sendLiteral(session, text string) error {
	_, err := tmuxCmd("send-keys", "-t", session, "-l", text)
	return err
}

// setupPipePane sets up pipe-pane to redirect output to a file/FIFO
func setupPipePane(session, target string) error {
	// -o flag: only capture stdout (not stdin)
	cmd := fmt.Sprintf("cat >> %s", target)
	_, err := tmuxCmd("pipe-pane", "-o", "-t", session, cmd)
	return err
}

// closePipePane closes the pipe-pane for a session
func closePipePane(session string) error {
	// Calling pipe-pane with no command closes it
	_, err := tmuxCmd("pipe-pane", "-t", session)
	return err
}

// killSession kills a tmux session
func killSession(session string) error {
	// Don't return error if session doesn't exist
	cmd := exec.Command("tmux", "kill-session", "-t", session)
	cmd.Run()
	return nil
}

// sessionExists checks if a tmux session exists
func sessionExists(name string) bool {
	cmd := exec.Command("tmux", "has-session", "-t", name)
	return cmd.Run() == nil
}

// getSessionPID returns the PID of the process running in the tmux session
func getSessionPID(session string) (int, error) {
	// Get the pane PID
	out, err := tmuxCmd("display-message", "-t", session, "-p", "#{pane_pid}")
	if err != nil {
		return 0, err
	}
	var pid int
	_, err = fmt.Sscanf(strings.TrimSpace(out), "%d", &pid)
	return pid, err
}

// makeNonBlocking sets a file descriptor to non-blocking mode
func makeNonBlocking(fd int) error {
	return syscall.SetNonblock(fd, true)
}
