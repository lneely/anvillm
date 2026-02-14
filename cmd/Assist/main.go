// Assist - Acme interface for AnviLLM
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"9fans.net/go/acme"
	"9fans.net/go/plan9"
	"9fans.net/go/plan9/client"
)

const windowName = "/AnviLLM/"

// Terminal to use for tmux attach (configurable via environment or flag)
var terminalCommand = getTerminalCommand()

func getTerminalCommand() string {
	if term := os.Getenv("ANVILLM_TERMINAL"); term != "" {
		return term
	}
	return "foot" // default
}

type SessionInfo struct {
	ID      string
	Backend string
	State   string
	Alias   string
	Cwd     string
	Pid     int
	WinID   int
}

var (
	fs *client.Fsys
	// Track window IDs for prompt windows (client-side state)
	promptWindows   = make(map[string]int) // session ID -> window ID
	promptWindowsMu sync.RWMutex            // protects promptWindows map

	// Compile regex patterns once at startup
	sessionIDRegex = regexp.MustCompile(`^[a-f0-9]{8}$`)
	aliasRegex     = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
)

func main() {
	flag.Parse()

	// Connect to anvilsrv via 9P, auto-starting if needed
	var err error
	fs, err = connectToServer()
	if err != nil {
		// Try to start anvilsrv automatically
		log.Printf("anvilsrv not running, attempting to start...")
		startCmd := exec.Command("anvilsrv", "start")
		if err := startCmd.Run(); err != nil {
			log.Fatalf("Failed to start anvilsrv: %v", err)
		}

		// Wait a moment for daemon to initialize
		for i := 0; i < 20; i++ {
			fs, err = connectToServer()
			if err == nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}

		if err != nil {
			log.Fatalf("Failed to connect to anvilsrv after starting: %v", err)
		}
	}
	defer fs.Close()

	// Start notification daemon in background
	notifyCmd := exec.Command("anvillm-notify")
	if err := notifyCmd.Start(); err != nil {
		log.Printf("Warning: Failed to start anvillm-notify: %v", err)
	}
	defer func() {
		if notifyCmd.Process != nil {
			notifyCmd.Process.Kill()
		}
	}()

	w, err := acme.New()
	if err != nil {
		log.Fatal(err)
	}
	defer w.CloseFiles()

	w.Name(windowName)
	w.Write("tag", []byte("Get Attach Stop Restart Kill Refresh Alias Context "))
	refreshList(w)
	w.Ctl("clean")

	// Event loop
	for e := range w.EventChan() {
		switch e.C2 {
		case 'x', 'X':
			cmd := string(e.Text)
			arg := strings.TrimSpace(string(e.Arg))

			// Fire-and-forget: B2 on session ID sends selected text as prompt
			// In a 2-1 chord, the selection comes through as e.Arg
			if sessionIDRegex.MatchString(cmd) {
				if len(e.Arg) > 0 {
					go sendPrompt(cmd, string(e.Arg))
				}
				continue
			}

			// Parse commands with arguments (e.g., "Kiro /path" -> cmd="Kiro", arg="/path")
			cmd, arg = parseCommand(cmd, arg)

			switch cmd {
			case "Kiro":
				if arg == "" {
					fmt.Fprintf(os.Stderr, "Error: Kiro requires a path argument\n")
					continue
				}
				if err := createSession("kiro-cli", arg); err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					continue
				}
				refreshList(w)
			case "Claude":
				if arg == "" {
					fmt.Fprintf(os.Stderr, "Error: Claude requires a path argument\n")
					continue
				}
				if err := createSession("claude", arg); err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					continue
				}
				refreshList(w)
			case "Stop":
				if arg == "" {
					fmt.Fprintf(os.Stderr, "Usage: Stop <session-id>\n")
					continue
				}
				if err := controlSession(arg, "stop"); err != nil {
					fmt.Fprintf(os.Stderr, "Failed to stop session: %v\n", err)
					continue
				}
				refreshList(w)
			case "Restart":
				if arg == "" {
					fmt.Fprintf(os.Stderr, "Usage: Restart <session-id>\n")
					continue
				}
				if err := controlSession(arg, "restart"); err != nil {
					fmt.Fprintf(os.Stderr, "Failed to restart session: %v\n", err)
					continue
				}
				refreshList(w)
			case "Kill":
				if arg == "" {
					fmt.Fprintf(os.Stderr, "Usage: Kill <session-id>\n")
					continue
				}
				if err := controlSession(arg, "kill"); err != nil {
					fmt.Fprintf(os.Stderr, "Failed to kill session: %v\n", err)
					continue
				}
				refreshList(w)
			case "Attach":
				if arg == "" {
					fmt.Fprintf(os.Stderr, "Usage: Attach <session-id>\n")
					continue
				}
				if err := attachSession(arg); err != nil {
					fmt.Fprintf(os.Stderr, "Failed to attach: %v\n", err)
				}
			case "Get":
				refreshList(w)
			case "Alias":
				parts := strings.Fields(arg)
				if len(parts) < 2 {
					fmt.Fprintf(os.Stderr, "Usage: Alias <session-id> <name>\n")
					continue
				}
				id := parts[0]
				alias := parts[1]
				if !aliasRegex.MatchString(alias) {
					fmt.Fprintf(os.Stderr, "Invalid alias: must match [A-Za-z0-9_-]+\n")
					continue
				}
				if err := setAlias(id, alias); err != nil {
					fmt.Fprintf(os.Stderr, "Failed to set alias: %v\n", err)
					continue
				}
				// Rename prompt window if it exists
				promptWindowsMu.RLock()
				winID, ok := promptWindows[id]
				promptWindowsMu.RUnlock()
				if ok {
					if aw, err := acme.Open(winID, nil); err == nil {
						sess, _ := getSession(id)
						displayName := alias
						if displayName == "" {
							displayName = id
						}
						aw.Name(filepath.Join(sess.Cwd, fmt.Sprintf("+Prompt.%s", displayName)))
						aw.CloseFiles()
					}
				}
				refreshList(w)
			case "Refresh":
				if arg == "" {
					// Refresh all sessions
					sessions, _ := listSessions()
					for _, sess := range sessions {
						controlSession(sess.ID, "refresh")
					}
				} else {
					if err := controlSession(arg, "refresh"); err != nil {
						fmt.Fprintf(os.Stderr, "Failed to refresh session: %v\n", err)
						continue
					}
				}
				refreshList(w)
			case "Context":
				if arg == "" {
					fmt.Fprintf(os.Stderr, "Usage: Context <session-id>\n")
					continue
				}
				sess, err := getSession(arg)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Session not found: %s\n", arg)
					continue
				}
				if err := openContextWindow(sess); err != nil {
					fmt.Fprintf(os.Stderr, "Error opening context window: %v\n", err)
				}

			default:
				w.WriteEvent(e)
			}
		case 'l', 'L':
			text := strings.TrimSpace(string(e.Text))
			if sessionIDRegex.MatchString(text) {
				// Try to open/focus prompt window
				promptWindowsMu.RLock()
				winID, ok := promptWindows[text]
				promptWindowsMu.RUnlock()
				if ok {
					if aw, err := acme.Open(winID, nil); err == nil {
						aw.Ctl("show")
						aw.CloseFiles()
					} else {
						// Window died, open new one
						sess, _ := getSession(text)
						if sess != nil {
							openPromptWindow(sess)
						}
					}
				} else {
					// Open new prompt window
					sess, _ := getSession(text)
					if sess != nil {
						openPromptWindow(sess)
					}
				}
			} else {
				w.WriteEvent(e)
			}
		default:
			w.WriteEvent(e)
		}
	}
}

// parseCommand extracts command and argument from input text
// Handles cases like "Kiro /path" -> ("Kiro", "/path")
func parseCommand(cmd, arg string) (string, string) {
	commandsWithArgs := []string{"Kiro", "Claude", "Stop", "Restart", "Kill", "Alias", "Context"}

	for _, cmdName := range commandsWithArgs {
		prefix := cmdName + " "
		if strings.HasPrefix(cmd, prefix) {
			return cmdName, strings.TrimPrefix(cmd, prefix)
		}
	}

	return cmd, arg
}

func connectToServer() (*client.Fsys, error) {
	ns := client.Namespace()
	if ns == "" {
		return nil, fmt.Errorf("no namespace")
	}

	// MountService expects just the service name, it adds the namespace automatically
	return client.MountService("agent")
}

func createSession(backend, cwd string) error {
	// Validate and clean the path
	cleanPath := filepath.Clean(cwd)

	// Ensure it's an absolute path
	if !filepath.IsAbs(cleanPath) {
		var err error
		cleanPath, err = filepath.Abs(cleanPath)
		if err != nil {
			return fmt.Errorf("invalid path: %v", err)
		}
	}

	// Verify the directory exists
	if info, err := os.Stat(cleanPath); err != nil {
		return fmt.Errorf("path does not exist: %v", err)
	} else if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", cleanPath)
	}

	fid, err := fs.Open("ctl", plan9.OWRITE)
	if err != nil {
		return err
	}
	defer fid.Close()

	cmd := fmt.Sprintf("new %s %s", backend, cleanPath)
	_, err = fid.Write([]byte(cmd))
	return err
}

func controlSession(id, cmd string) error {
	path := filepath.Join(id, "ctl")
	fid, err := fs.Open(path, plan9.OWRITE)
	if err != nil {
		return err
	}
	defer fid.Close()

	_, err = fid.Write([]byte(cmd))
	return err
}

func sendPrompt(id, prompt string) error {
	path := filepath.Join(id, "in")
	fid, err := fs.Open(path, plan9.OWRITE)
	if err != nil {
		return err
	}
	defer fid.Close()

	_, err = fid.Write([]byte(prompt))
	return err
}

func setAlias(id, alias string) error {
	path := filepath.Join(id, "alias")
	fid, err := fs.Open(path, plan9.OWRITE)
	if err != nil {
		return err
	}
	defer fid.Close()

	_, err = fid.Write([]byte(alias))
	return err
}

func getSession(id string) (*SessionInfo, error) {
	// Read session metadata from 9P files
	sess := &SessionInfo{ID: id}

	// Read backend
	if data, err := readFile(filepath.Join(id, "backend")); err == nil {
		sess.Backend = strings.TrimSpace(string(data))
	}

	// Read state
	if data, err := readFile(filepath.Join(id, "state")); err == nil {
		sess.State = strings.TrimSpace(string(data))
	}

	// Read alias
	if data, err := readFile(filepath.Join(id, "alias")); err == nil {
		sess.Alias = strings.TrimSpace(string(data))
	}

	// Read cwd
	if data, err := readFile(filepath.Join(id, "cwd")); err == nil {
		sess.Cwd = strings.TrimSpace(string(data))
	}

	// Read pid
	if data, err := readFile(filepath.Join(id, "pid")); err == nil {
		fmt.Sscanf(string(data), "%d", &sess.Pid)
	}

	return sess, nil
}

func listSessions() ([]*SessionInfo, error) {
	data, err := readFile("list")
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(data), "\n")
	var sessions []*SessionInfo

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse: id backend state alias cwd
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}

		sess := &SessionInfo{
			ID:      fields[0],
			Backend: fields[1],
			State:   fields[2],
			Alias:   fields[3],
			Cwd:     strings.Join(fields[4:], " "),
		}
		if sess.Alias == "-" {
			sess.Alias = ""
		}
		sessions = append(sessions, sess)
	}

	return sessions, nil
}

func readFile(path string) ([]byte, error) {
	fid, err := fs.Open(path, plan9.OREAD)
	if err != nil {
		return nil, err
	}
	defer fid.Close()

	var buf []byte
	tmp := make([]byte, 8192)
	for {
		n, err := fid.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil {
			break
		}
	}
	return buf, nil
}

func writeFile(path string, data []byte) error {
	fid, err := fs.Open(path, plan9.OWRITE)
	if err != nil {
		return err
	}
	defer fid.Close()

	_, err = fid.Write(data)
	return err
}

func refreshList(w *acme.Win) {
	sessions, err := listSessions()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to list sessions: %v\n", err)
		return
	}

	var buf strings.Builder
	buf.WriteString("Backends: [Kiro] [Claude]\n\n")
	buf.WriteString(fmt.Sprintf("%-5s %-10s %-9s %-16s %s\n", "ID", "Backend", "State", "Alias", "Cwd"))
	buf.WriteString(fmt.Sprintf("%-5s %-10s %-9s %-16s %s\n", "-----", "----------", "---------", "----------------", strings.Repeat("-", 40)))

	for _, sess := range sessions {
		// Check if PID is alive (server handles this via Refresh)
		if sess.Pid != 0 {
			if err := syscall.Kill(sess.Pid, 0); err != nil {
				// Only refresh if process doesn't exist (ESRCH)
				// Ignore permission errors (EPERM) - process exists but we can't signal it
				if err == syscall.ESRCH {
					// Process died, trigger refresh
					fmt.Fprintf(os.Stderr, "Warning: session %s PID %d died, refreshing\n", sess.ID, sess.Pid)
					controlSession(sess.ID, "refresh")
					// Re-read session info
					if updated, err := getSession(sess.ID); err == nil {
						sess = updated
					}
				}
			}
		}

		alias := sess.Alias
		if alias == "" {
			alias = "-"
		}
		buf.WriteString(fmt.Sprintf("%-5s %-10s %-9s %-16s %s\n", sess.ID, sess.Backend, sess.State, alias, sess.Cwd))
	}

	w.Addr(",")
	w.Write("data", []byte(buf.String()))
	w.Ctl("clean")
}

func openPromptWindow(sess *SessionInfo) (*acme.Win, error) {
	displayName := sess.Alias
	if displayName == "" {
		displayName = sess.ID
	}
	name := filepath.Join(sess.Cwd, fmt.Sprintf("+Prompt.%s", displayName))

	w, err := acme.New()
	if err != nil {
		return nil, err
	}
	w.Name(name)
	w.Write("tag", []byte("Send "))
	w.Ctl("clean")

	// Track window ID client-side
	promptWindowsMu.Lock()
	promptWindows[sess.ID] = w.ID()
	promptWindowsMu.Unlock()

	go handlePromptWindow(w, sess)
	return w, nil
}

func handlePromptWindow(w *acme.Win, sess *SessionInfo) {
	defer w.CloseFiles()
	defer func() {
		promptWindowsMu.Lock()
		delete(promptWindows, sess.ID)
		promptWindowsMu.Unlock()
	}()

	for e := range w.EventChan() {
		if (e.C2 == 'x' || e.C2 == 'X') && string(e.Text) == "Send" {
			body, err := w.ReadAll("body")
			if err != nil {
				continue
			}
			prompt := strings.TrimSpace(string(body))
			if prompt != "" {
				go sendPrompt(sess.ID, prompt)
				w.Ctl("delete")
				return
			}
		} else {
			w.WriteEvent(e)
		}
	}
}

func openContextWindow(sess *SessionInfo) error {
	w, err := acme.New()
	if err != nil {
		return err
	}
	w.Name(fmt.Sprintf("/AnviLLM/%s/context", sess.ID))
	w.Write("tag", []byte("Put "))

	// Load existing context
	if data, err := readFile(filepath.Join(sess.ID, "context")); err == nil {
		w.Write("body", data)
	}
	w.Ctl("clean")

	go handleContextWindow(w, sess)
	return nil
}

func handleContextWindow(w *acme.Win, sess *SessionInfo) {
	defer w.CloseFiles()

	for e := range w.EventChan() {
		if e.C2 == 'x' || e.C2 == 'X' {
			if string(e.Text) == "Put" {
				body, err := w.ReadAll("body")
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error reading body: %v\n", err)
					continue
				}
				path := filepath.Join(sess.ID, "context")
				if err := writeFile(path, body); err != nil {
					fmt.Fprintf(os.Stderr, "Error writing context: %v\n", err)
				} else {
					w.Ctl("clean")
					fmt.Printf("Context updated for session %s\n", sess.ID)
				}
				continue
			}
		}
		w.WriteEvent(e)
	}
}

func attachSession(id string) error {
	sess, err := getSession(id)
	if err != nil {
		return err
	}

	// Read tmux session/window from session metadata
	// This requires reading the extra metadata - for now, construct from backend name
	// The server stores tmux_session and tmux_window in Extra, but we can't access that via 9P yet
	// For now, assume tmux session is "anvillm-{backend}" and window is the session ID
	tmuxSession := fmt.Sprintf("anvillm-%s", sess.Backend)
	tmuxWindow := id

	go func() {
		target := fmt.Sprintf("%s:%s", tmuxSession, tmuxWindow)
		cmd := exec.Command(terminalCommand, "-e", "tmux", "attach", "-t", target)
		if err := cmd.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to launch terminal: %v\n", err)
		}
	}()

	return nil
}
