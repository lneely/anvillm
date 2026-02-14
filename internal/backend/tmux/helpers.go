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

// createWindow creates a new window in an existing tmux session
func createWindow(session, windowName string) error {
	_, err := tmuxCmd("new-window", "-t", session, "-n", windowName)
	return err
}

// killWindow kills a window in a tmux session
func killWindow(session, windowName string) error {
	target := fmt.Sprintf("%s:%s", session, windowName)
	cmd := exec.Command("tmux", "kill-window", "-t", target)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tmux kill-window -t %s: %w", target, err)
	}
	return nil
}

// windowTarget constructs a tmux target for a window in a session
func windowTarget(session, windowName string) string {
	return fmt.Sprintf("%s:%s", session, windowName)
}

// setEnvironment sets an environment variable in the tmux session or window
func setEnvironment(target, key, value string) error {
	_, err := tmuxCmd("set-environment", "-t", target, key, value)
	return err
}

// sendKeys sends keys to a tmux target (session or session:window)
// Special keys: C-m (Enter), C-c (Ctrl+C), etc.
func sendKeys(target string, keys ...string) error {
	args := []string{"send-keys", "-t", target}
	args = append(args, keys...)
	_, err := tmuxCmd(args...)
	return err
}

// sendLiteral sends literal text to a tmux target (doesn't interpret special chars)
func sendLiteral(target, text string) error {
	_, err := tmuxCmd("send-keys", "-t", target, "-l", text)
	return err
}

// setupPipePane sets up pipe-pane to redirect output to a file/FIFO
func setupPipePane(tmuxTarget, fifoPath string) error {
	// -o flag: only capture stdout (not stdin)
	cmd := fmt.Sprintf("cat >> %s", fifoPath)
	_, err := tmuxCmd("pipe-pane", "-o", "-t", tmuxTarget, cmd)
	return err
}

// closePipePane closes the pipe-pane for a target
func closePipePane(target string) error {
	// Calling pipe-pane with no command closes it
	_, err := tmuxCmd("pipe-pane", "-t", target)
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

// windowExists checks if a window exists in a tmux session
func windowExists(session, windowName string) bool {
	target := fmt.Sprintf("%s:%s", session, windowName)
	// Try to list the window - if it doesn't exist, this will fail
	cmd := exec.Command("tmux", "list-windows", "-t", target, "-F", "#{window_name}")
	return cmd.Run() == nil
}

// getPanePID returns the PID of the process running in a tmux pane
func getPanePID(target string) (int, error) {
	// Get the pane PID
	out, err := tmuxCmd("display-message", "-t", target, "-p", "#{pane_pid}")
	if err != nil {
		return 0, err
	}
	var pid int
	_, err = fmt.Sscanf(strings.TrimSpace(out), "%d", &pid)
	return pid, err
}

// FindKiroChatPID traverses the process tree to find kiro-cli-chat
func FindKiroChatPID(panePID int) int {
	// pane (kiro-cli-term) -> bash -> kiro-cli -> kiro-cli-chat
	bashPID := FindChildByName(panePID, "bash")
	if bashPID == 0 {
		return 0
	}
	kiroPID := FindChildByName(bashPID, "kiro-cli")
	if kiroPID == 0 {
		return 0
	}
	return FindChildByName(kiroPID, "kiro-cli-chat")
}

// FindChildByName finds a child process with the given name
func FindChildByName(parentPID int, name string) int {
	cmd := exec.Command("pgrep", "-P", fmt.Sprintf("%d", parentPID), "-x", name)
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	var pid int
	fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &pid)
	return pid
}

// makeNonBlocking sets a file descriptor to non-blocking mode
func makeNonBlocking(fd int) error {
	return syscall.SetNonblock(fd, true)
}
