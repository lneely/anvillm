// Assist - Acme interface for AnviLLM
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
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
			log.Printf("ERROR: Failed to start anvilsrv: %v", err)
			log.Printf("Continuing without daemon connection. Use 'Daemon' command to manage server.")
		} else {
			// Wait a moment for daemon to initialize
			for i := 0; i < 20; i++ {
				fs, err = connectToServer()
				if err == nil {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}

			if err != nil {
				log.Printf("ERROR: Failed to connect to anvilsrv after starting: %v", err)
				log.Printf("Continuing without daemon connection. Use 'Daemon' command to manage server.")
			}
		}
	}
	if fs != nil {
		defer fs.Close()
	}

	w, err := acme.New()
	if err != nil {
		log.Fatal(err)
	}
	defer w.CloseFiles()

	w.Name(windowName)
	w.Write("tag", []byte("Get Attach Stop Restart Kill Alias Context Daemon Inbox Archive Tasks "))
	refreshList(w)
	w.Ctl("clean")

	// Event loop
	for e := range w.EventChan() {
		switch e.C2 {
		case 'x', 'X':
			cmd := string(e.Text)
			arg := strings.TrimSpace(string(e.Arg))

			// Handle session ID clicks
			if sessionIDRegex.MatchString(cmd) {
				if len(e.Arg) > 0 {
					// Fire-and-forget: B2 on session ID with selected text sends prompt
					go func(id, prompt string) {
						if err := sendPrompt(id, prompt); err != nil {
							fmt.Fprintf(os.Stderr, "Failed to send prompt: %v\n", err)
						}
					}(cmd, string(e.Arg))
				} else {
					// Middle-click on session ID without selection attaches to tmux
					if err := attachSession(cmd); err != nil {
						fmt.Fprintf(os.Stderr, "Failed to attach: %v\n", err)
					}
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
			case "Ollama":
				if arg == "" {
					fmt.Fprintf(os.Stderr, "Error: Ollama requires a path argument\n")
					continue
				}
				if err := createSession("ollama", arg); err != nil {
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

			case "Daemon":
				if err := openDaemonWindow(); err != nil {
					fmt.Fprintf(os.Stderr, "Error opening daemon window: %v\n", err)
				}
			case "Inbox":
				owner := "user"
				if arg != "" {
					owner = arg
				}
				if err := openInboxWindow(owner); err != nil {
					fmt.Fprintf(os.Stderr, "Error opening inbox window: %v\n", err)
				}
			case "Archive":
				owner := "user"
				if arg != "" {
					owner = arg
				}
				if err := openArchiveWindow(owner); err != nil {
					fmt.Fprintf(os.Stderr, "Error opening archive window: %v\n", err)
				}
			case "Tasks":
				if err := openTasksWindow(); err != nil {
					fmt.Fprintf(os.Stderr, "Error opening tasks window: %v\n", err)
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
	commandsWithArgs := []string{"Kiro", "Claude", "Ollama", "Stop", "Restart", "Kill", "Alias", "Context"}

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

func isConnected() bool {
	return fs != nil
}

func createSession(backend, cwd string) error {
	if !isConnected() {
		return fmt.Errorf("not connected to anvilsrv (use Daemon command to start server)")
	}
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
	if !isConnected() {
		return fmt.Errorf("not connected to anvilsrv")
	}
	path := filepath.Join(id, "ctl")
	fid, err := fs.Open(path, plan9.OWRITE)
	if err != nil {
		return err
	}
	defer fid.Close()

	_, err = fid.Write([]byte(cmd))
	return err
}

func isBeadID(text string) bool {
	// Match bead ID pattern: prefix-xxx or prefix-xxx.yyy (hierarchical)
	// Common prefixes: bd, task, bug, etc.
	matched, _ := regexp.MatchString(`^[a-zA-Z]+-[a-z0-9]+(\.[0-9]+)*$`, text)
	return matched
}

func sendPrompt(id, prompt string) error {
	if !isConnected() {
		return fmt.Errorf("not connected to anvilsrv")
	}

	// Check if prompt is a bead ID and construct execution prompt
	trimmedPrompt := strings.TrimSpace(prompt)
	if isBeadID(trimmedPrompt) {
		prompt = fmt.Sprintf("Load the beads skill, and work on bead %s.", trimmedPrompt)
	}

	// Create message JSON
	msg := map[string]interface{}{
		"to":      id,
		"type":    "PROMPT_REQUEST",
		"subject": "User prompt",
		"body":    prompt,
	}
	msgJSON, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Write to user mail
	path := "user/mail"

	fid, err := fs.Open(path, plan9.OWRITE)
	if err != nil {
		return fmt.Errorf("failed to open mail file: %w", err)
	}
	defer fid.Close()
	_, err = fid.Write(msgJSON)
	if err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	return nil
}

func setAlias(id, alias string) error {
	if !isConnected() {
		return fmt.Errorf("not connected to anvilsrv")
	}
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
	if !isConnected() {
		return nil, fmt.Errorf("not connected to anvilsrv")
	}
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
	if !isConnected() {
		return nil, fmt.Errorf("not connected to anvilsrv")
	}
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
	if !isConnected() {
		return fmt.Errorf("not connected to anvilsrv")
	}
	fid, err := fs.Open(path, plan9.OWRITE)
	if err != nil {
		return err
	}
	defer fid.Close()

	_, err = fid.Write(data)
	return err
}

func refreshList(w *acme.Win) {
	var buf strings.Builder
	buf.WriteString("Backends: [Kiro] [Claude] [Ollama]\n\n")

	if !isConnected() {
		buf.WriteString("Not connected to anvilsrv daemon.\n")
		buf.WriteString("Use 'Daemon' command to start the server.\n")
		w.Addr(",")
		w.Write("data", []byte(buf.String()))
		w.Ctl("clean")
		return
	}

	sessions, err := listSessions()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to list sessions: %v\n", err)
		buf.WriteString("Error listing sessions: " + err.Error() + "\n")
		w.Addr(",")
		w.Write("data", []byte(buf.String()))
		w.Ctl("clean")
		return
	}

	buf.WriteString(fmt.Sprintf("%-8s %-10s %-9s %-16s %s\n", "ID", "Backend", "State", "Alias", "Cwd"))
	buf.WriteString(fmt.Sprintf("%-8s %-10s %-9s %-16s %s\n", "--------", "----------", "---------", "----------------", strings.Repeat("-", 40)))

	for _, sess := range sessions {
		alias := sess.Alias
		if alias == "" {
			alias = "-"
		}
		buf.WriteString(fmt.Sprintf("%-8s %-10s %-9s %-16s %s\n", sess.ID, sess.Backend, sess.State, alias, sess.Cwd))
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
		cmd := string(e.Text)
		if (e.C2 == 'x' || e.C2 == 'X') && cmd == "Send" {
			body, err := w.ReadAll("body")
			if err != nil {
				continue
			}
			prompt := strings.TrimSpace(string(body))
			if prompt != "" {
				if err := sendPrompt(sess.ID, prompt); err != nil {
					fmt.Fprintf(os.Stderr, "Failed to send: %v\n", err)
					continue
				}
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
	// Read tmux session/window from the tmux file
	tmuxPath := filepath.Join(id, "tmux")
	data, err := readFile(tmuxPath)
	if err != nil {
		return fmt.Errorf("failed to read tmux target: %w", err)
	}

	target := strings.TrimSpace(string(data))
	if target == "" {
		return fmt.Errorf("session does not support attach")
	}

	// Check if there's already a tmux client running
	clientsCmd := exec.Command("tmux", "list-clients")
	clientsOutput, err := clientsCmd.Output()
	if err == nil && len(clientsOutput) > 0 {
		// There's an existing tmux client, switch to the target session/window
		go func() {
			cmd := exec.Command("tmux", "switch-client", "-t", target)
			if err := cmd.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to switch tmux client: %v\n", err)
			}
		}()
	} else {
		// No existing client, launch new terminal
		go func() {
			cmd := exec.Command(terminalCommand, "-e", "tmux", "attach", "-t", target)
			if err := cmd.Start(); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to launch terminal: %v\n", err)
			}
		}()
	}

	return nil
}

// openDaemonWindow opens the daemon management window
func openDaemonWindow() error {
	w, err := acme.New()
	if err != nil {
		return err
	}

	w.Name("/AnviLLM/Daemon")
	w.Write("tag", []byte("Get Stop Start Restart "))

	go handleDaemonWindow(w)
	return nil
}

func handleDaemonWindow(w *acme.Win) {
	defer w.CloseFiles()

	// Load and display current status
	refreshDaemonWindow(w)

	for e := range w.EventChan() {
		switch e.C2 {
		case 'x', 'X':
			cmd := string(e.Text)

			switch cmd {
			case "Get":
				refreshDaemonWindow(w)
			case "Stop":
				stopDaemon(w)
				time.Sleep(500 * time.Millisecond)
				refreshDaemonWindow(w)
			case "Start":
				startDaemon(w)
				time.Sleep(1 * time.Second)
				refreshDaemonWindow(w)
			case "Restart":
				stopDaemon(w)
				time.Sleep(500 * time.Millisecond)
				startDaemon(w)
				time.Sleep(1 * time.Second)
				refreshDaemonWindow(w)
			default:
				w.WriteEvent(e)
			}
		default:
			w.WriteEvent(e)
		}
	}
}

func refreshDaemonWindow(w *acme.Win) {
	var buf strings.Builder

	buf.WriteString("AnviLLM Daemon Status\n")
	buf.WriteString(strings.Repeat("=", 60) + "\n\n")

	// Check daemon status
	statusCmd := exec.Command("anvilsrv", "status")
	output, err := statusCmd.CombinedOutput()

	if err != nil {
		// Not running or error
		buf.WriteString("Status: NOT RUNNING\n")
		if len(output) > 0 {
			buf.WriteString(string(output))
		}
		buf.WriteString("\n")
	} else {
		// Running
		buf.WriteString("Status: RUNNING\n")
		buf.WriteString(string(output))
		buf.WriteString("\n")
	}

	// Check socket
	ns := client.Namespace()
	if ns != "" {
		sockPath := filepath.Join(ns, "agent")
		if _, err := os.Stat(sockPath); err == nil {
			buf.WriteString("9P Socket: " + sockPath + " (exists)\n")
		} else {
			buf.WriteString("9P Socket: " + sockPath + " (missing)\n")
		}
	}

	// Check connection
	if fs != nil {
		buf.WriteString("Connection: CONNECTED\n")
	} else {
		buf.WriteString("Connection: DISCONNECTED\n")
	}

	buf.WriteString("\n")
	buf.WriteString(strings.Repeat("-", 60) + "\n")
	buf.WriteString("Commands:\n")
	buf.WriteString("  Start   - Start the daemon (anvilsrv start)\n")
	buf.WriteString("  Stop    - Stop the daemon (anvilsrv stop)\n")
	buf.WriteString("  Restart - Restart the daemon\n")
	buf.WriteString("  Get     - Refresh this window\n\n")

	buf.WriteString("Note: Use 'anvilsrv fgstart' in terminal for debug logs.\n")

	w.Addr(",")
	w.Write("data", []byte(buf.String()))
	w.Ctl("clean")
}

func startDaemon(w *acme.Win) {
	w.Addr("$")
	w.Write("data", []byte("\nStarting daemon...\n"))

	cmd := exec.Command("anvilsrv", "start")
	output, err := cmd.CombinedOutput()

	if err != nil {
		w.Write("data", []byte(fmt.Sprintf("Error: %v\n%s\n", err, output)))
	} else {
		w.Write("data", []byte("Daemon started successfully\n"))
		if len(output) > 0 {
			w.Write("data", []byte(string(output)+"\n"))
		}

		// Try to reconnect
		time.Sleep(500 * time.Millisecond)
		if newFs, err := connectToServer(); err == nil {
			if fs != nil {
				fs.Close()
			}
			fs = newFs
			w.Write("data", []byte("Reconnected to daemon\n"))
		}
	}
}

func stopDaemon(w *acme.Win) {
	w.Addr("$")
	w.Write("data", []byte("\nStopping daemon...\n"))

	cmd := exec.Command("anvilsrv", "stop")
	output, err := cmd.CombinedOutput()

	if err != nil {
		w.Write("data", []byte(fmt.Sprintf("Error: %v\n%s\n", err, output)))
	} else {
		w.Write("data", []byte("Daemon stopped\n"))
		if len(output) > 0 {
			w.Write("data", []byte(string(output)+"\n"))
		}

		// Disconnect
		if fs != nil {
			fs.Close()
			fs = nil
		}
	}
}

type Message struct {
	ID        string `json:"id"`
	From      string `json:"from"`
	To        string `json:"to"`
	Type      string `json:"type"`
	Subject   string `json:"subject"`
	Body      string `json:"body"`
	Timestamp int64  `json:"timestamp"`
}

func openInboxWindow(owner string) error {
	w, err := acme.New()
	if err != nil {
		return err
	}

	if owner == "user" {
		w.Name("/AnviLLM/inbox")
	} else {
		w.Name(fmt.Sprintf("/AnviLLM/%s/inbox", owner))
	}
	w.Write("tag", []byte("Get "))

	go handleInboxWindow(w, owner)
	return nil
}

func handleInboxWindow(w *acme.Win, owner string) {
	defer w.CloseFiles()

	folder := owner + "/inbox"
	refreshMailboxWindow(w, folder, "Inbox")

	for e := range w.EventChan() {
		switch e.C2 {
		case 'x', 'X':
			if string(e.Text) == "Get" {
				refreshMailboxWindow(w, folder, "Inbox")
			} else {
				w.WriteEvent(e)
			}
		case 'l', 'L':
			text := strings.TrimSpace(string(e.Text))
			if isHexString(text) {
				openMessageWindowByPrefix(text, folder)
			} else {
				w.WriteEvent(e)
			}
		default:
			w.WriteEvent(e)
		}
	}
}

func refreshMailboxWindow(w *acme.Win, folder, title string) {
	messages, _, err := listMessages(folder)
	if err != nil {
		w.Addr(",")
		w.Write("data", []byte(fmt.Sprintf("Error reading %s: %v\n", title, err)))
		w.Ctl("clean")
		return
	}

	// Sort by timestamp, newest first
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].Timestamp > messages[j].Timestamp
	})

	var buf strings.Builder
	buf.WriteString(fmt.Sprintf("%s %s\n", folder, title))
	buf.WriteString(strings.Repeat("=", 120) + "\n\n")
	buf.WriteString(fmt.Sprintf("%-10s %-20s %-12s %-18s %s\n", "ID", "Date", "From", "Type", "Subject"))
	buf.WriteString(fmt.Sprintf("%-10s %-20s %-12s %-18s %s\n", "----------", "--------------------", "------------", "------------------", strings.Repeat("-", 40)))

	for _, msg := range messages {
		from := msg.From
		if from == "" {
			from = "-"
		}
		shortID := shortUUID(msg.ID)
		dateStr := formatTimestamp(msg.Timestamp)
		buf.WriteString(fmt.Sprintf("%-10s %-20s %-12s %-18s %s\n", shortID, dateStr, from, msg.Type, msg.Subject))
	}

	w.Addr(",")
	w.Write("data", []byte(buf.String()))
	w.Ctl("clean")
}

func shortUUID(id string) string {
	if idx := strings.Index(id, "-"); idx > 0 {
		return id[:idx]
	}
	return id
}

func expandUUID(shortID string, messages []Message) string {
	for _, msg := range messages {
		if strings.HasPrefix(msg.ID, shortID) {
			return msg.ID
		}
	}
	return shortID
}

func openArchiveWindow(owner string) error {
	w, err := acme.New()
	if err != nil {
		return err
	}

	if owner == "user" {
		w.Name("/AnviLLM/archive")
	} else {
		w.Name(fmt.Sprintf("/AnviLLM/%s/archive", owner))
	}
	w.Write("tag", []byte("Get "))

	go handleArchiveWindow(w, owner)
	return nil
}

func handleArchiveWindow(w *acme.Win, owner string) {
	defer w.CloseFiles()

	folder := owner + "/completed"
	refreshMailboxWindow(w, folder, "Archive")

	for e := range w.EventChan() {
		switch e.C2 {
		case 'x', 'X':
			if string(e.Text) == "Get" {
				refreshMailboxWindow(w, folder, "Archive")
			} else {
				w.WriteEvent(e)
			}
		case 'l', 'L':
			text := strings.TrimSpace(string(e.Text))
			if isHexString(text) {
				openMessageWindowByPrefix(text, folder)
			} else {
				w.WriteEvent(e)
			}
		default:
			w.WriteEvent(e)
		}
	}
}

func listInboxMessages() ([]Message, []string, error) {
	return listMessages("user/inbox")
}

func listArchiveMessages() ([]Message, []string, error) {
	return listMessages("user/completed")
}

func listMessages(folder string) ([]Message, []string, error) {
	if !isConnected() {
		return nil, nil, fmt.Errorf("not connected to anvilsrv")
	}

	fid, err := fs.Open(folder, plan9.OREAD)
	if err != nil {
		return nil, nil, err
	}
	defer fid.Close()

	var filenames []string
	for {
		dirs, err := fid.Dirread()
		if err != nil || len(dirs) == 0 {
			break
		}
		for _, d := range dirs {
			if strings.HasSuffix(d.Name, ".json") {
				filenames = append(filenames, d.Name)
			}
		}
	}

	var messages []Message
	for _, filename := range filenames {
		data, err := readFile(filepath.Join(folder, filename))
		if err != nil {
			continue
		}
		var msg Message
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		messages = append(messages, msg)
	}

	return messages, filenames, nil
}

func extractTimestamp(filename string) int64 {
	var ts int64
	parts := strings.Split(strings.TrimSuffix(filename, ".json"), "-")
	if len(parts) == 2 {
		fmt.Sscanf(parts[1], "%d", &ts)
	}
	return ts
}

func formatTimestamp(ts int64) string {
	t := time.Unix(ts, 0)
	return t.Format("02-Jan-2006 15:04:05")
}

func parseIndex(s string) int {
	var idx int
	fmt.Sscanf(s, "%d", &idx)
	return idx
}

func isUUID(s string) bool {
	// Simple UUID check: 36 chars with hyphens at positions 8, 13, 18, 23
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func isHexString(s string) bool {
	if len(s) < 4 || len(s) > 36 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func openMessageWindowByPrefix(prefix, folder string) error {
	messages, _, err := listMessages(folder)
	if err != nil {
		return err
	}

	for _, m := range messages {
		if strings.HasPrefix(m.ID, prefix) {
			return openMessageWindowByID(m.ID, folder)
		}
	}
	return fmt.Errorf("message not found: %s", prefix)
}

func openMessageWindowByID(msgID, folder string) error {
	messages, filenames, err := listMessages(folder)
	if err != nil {
		return err
	}

	var msg *Message
	var filename string
	for i, m := range messages {
		if m.ID == msgID {
			msg = &messages[i]
			filename = filenames[i]
			break
		}
	}
	if msg == nil {
		return fmt.Errorf("message not found: %s", msgID)
	}

	w, err := acme.New()
	if err != nil {
		return err
	}

	windowPath := "inbox"
	if folder == "user/completed" {
		windowPath = "archive"
	}
	w.Name(fmt.Sprintf("/AnviLLM/%s/%s", windowPath, filename))
	w.Write("tag", []byte("Reply Archive "))

	var buf strings.Builder
	buf.WriteString(fmt.Sprintf("From: %s\n", msg.From))
	buf.WriteString(fmt.Sprintf("To: %s\n", msg.To))
	buf.WriteString(fmt.Sprintf("Type: %s\n", msg.Type))
	buf.WriteString(fmt.Sprintf("Subject: %s\n", msg.Subject))
	buf.WriteString(fmt.Sprintf("Date: %s\n", formatTimestamp(extractTimestamp(filename))))
	buf.WriteString("\n")
	buf.WriteString(msg.Body)

	w.Write("body", []byte(buf.String()))
	w.Ctl("clean")

	go handleMessageWindow(w, msg, filename)
	return nil
}

func openMessageWindow(idx int) error {
	messages, filenames, err := listInboxMessages()
	if err != nil {
		return err
	}

	if idx < 1 || idx > len(messages) {
		return fmt.Errorf("invalid index: %d", idx)
	}

	filename := filenames[idx-1]
	msg := messages[idx-1]

	w, err := acme.New()
	if err != nil {
		return err
	}

	w.Name(fmt.Sprintf("/AnviLLM/inbox/%s", filename))
	w.Write("tag", []byte("Reply Archive "))

	var buf strings.Builder
	buf.WriteString(fmt.Sprintf("From: %s\n", msg.From))
	buf.WriteString(fmt.Sprintf("To: %s\n", msg.To))
	buf.WriteString(fmt.Sprintf("Type: %s\n", msg.Type))
	buf.WriteString(fmt.Sprintf("Subject: %s\n", msg.Subject))
	buf.WriteString(fmt.Sprintf("Date: %s\n", formatTimestamp(extractTimestamp(filename))))
	buf.WriteString("\n")
	buf.WriteString(msg.Body)

	w.Write("body", []byte(buf.String()))
	w.Ctl("clean")

	go handleMessageWindow(w, &msg, filename)
	return nil
}

func handleMessageWindow(w *acme.Win, msg *Message, filename string) {
	defer w.CloseFiles()

	for e := range w.EventChan() {
		if (e.C2 == 'x' || e.C2 == 'X') && string(e.Text) == "Reply" {
			openReplyWindow(msg)
		} else if (e.C2 == 'x' || e.C2 == 'X') && string(e.Text) == "Archive" {
			if err := archiveMessage(msg.ID); err != nil {
				w.Fprintf("errors", "Archive failed: %v\n", err)
			} else {
				refreshInboxWindowByName()
				w.Ctl("delete")
			}
		} else {
			w.WriteEvent(e)
		}
	}
}

func openReplyWindow(originalMsg *Message) error {
	w, err := acme.New()
	if err != nil {
		return err
	}

	replyType := getReplyType(originalMsg.Type)
	replySubject := fmt.Sprintf("Re: %s", originalMsg.Subject)

	w.Name(fmt.Sprintf("/AnviLLM/reply/%s", originalMsg.From))
	w.Write("tag", []byte("Send "))
	w.Ctl("clean")

	go handleReplyWindow(w, originalMsg.From, replyType, replySubject)
	return nil
}

func refreshInboxWindowByName() {
	wins, err := acme.Windows()
	if err != nil {
		return
	}
	for _, info := range wins {
		if info.Name == "/AnviLLM/inbox" {
			w, err := acme.Open(info.ID, nil)
			if err != nil {
				continue
			}
			refreshMailboxWindow(w, "user/inbox", "Inbox")
			w.CloseFiles()
			return
		}
	}
}

func archiveMessage(msgID string) error {
	if !isConnected() {
		return fmt.Errorf("not connected to anvilsrv")
	}

	fid, err := fs.Open("user/ctl", plan9.OWRITE)
	if err != nil {
		return err
	}
	defer fid.Close()

	ctlMsg := fmt.Sprintf("complete %s", msgID)
	_, err = fid.Write([]byte(ctlMsg))
	return err
}

func getReplyType(msgType string) string {
	switch msgType {
	case "QUERY_REQUEST":
		return "QUERY_RESPONSE"
	case "REVIEW_REQUEST":
		return "REVIEW_RESPONSE"
	case "APPROVAL_REQUEST":
		return "APPROVAL_RESPONSE"
	default:
		return "PROMPT_REQUEST"
	}
}

func handleReplyWindow(w *acme.Win, to, msgType, subject string) {
	defer w.CloseFiles()

	for e := range w.EventChan() {
		if (e.C2 == 'x' || e.C2 == 'X') && string(e.Text) == "Send" {
			body, err := w.ReadAll("body")
			if err != nil {
				continue
			}
			prompt := strings.TrimSpace(string(body))
			if prompt != "" {
				if err := sendReply(to, msgType, subject, prompt); err != nil {
					fmt.Fprintf(os.Stderr, "Failed to send reply: %v\n", err)
					continue
				}
				w.Ctl("delete")
				return
			}
		} else {
			w.WriteEvent(e)
		}
	}
}

func sendReply(to, msgType, subject, body string) error {
	if !isConnected() {
		return fmt.Errorf("not connected to anvilsrv")
	}

	msg := map[string]interface{}{
		"to":      to,
		"type":    msgType,
		"subject": subject,
		"body":    body,
	}
	msgJSON, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	path := "user/mail"

	fid, err := fs.Open(path, plan9.OWRITE)
	if err != nil {
		return fmt.Errorf("failed to open mail file: %w", err)
	}
	defer fid.Close()

	if _, err := fid.Write(msgJSON); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	return nil
}

type Bead struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Status      string   `json:"status"`
	Assignee    string   `json:"assignee"`
	Priority    int      `json:"priority"`
	Blockers    []string `json:"blockers,omitempty"`
}

func openTasksWindow() error {
	w, err := acme.New()
	if err != nil {
		return err
	}

	w.Name("/AnviLLM/Tasks")
	w.Write("tag", []byte("Get New Remove "))

	go handleTasksWindow(w)
	return nil
}

func handleTasksWindow(w *acme.Win) {
	defer w.CloseFiles()

	var beads []Bead
	statusFilter := ""
	beads, _ = listBeadsWithFilter(statusFilter)
	refreshTasksWindowWithBeads(w, beads)

	for e := range w.EventChan() {
		switch e.C2 {
		case 'x', 'X':
			cmd := string(e.Text)
			arg := strings.TrimSpace(string(e.Arg))
			switch cmd {
			case "Get":
				beads, _ = listBeadsWithFilter(statusFilter)
				refreshTasksWindowWithBeads(w, beads)
			case "New":
				if err := openNewBeadWindow(""); err != nil {
					fmt.Fprintf(os.Stderr, "Error opening new bead window: %v\n", err)
				}
			case "Remove":
				if arg == "" {
					fmt.Fprintf(os.Stderr, "Usage: select bead ID, then Remove\n")
				} else if err := deleteBead(arg); err != nil {
					fmt.Fprintf(os.Stderr, "Error deleting bead: %v\n", err)
				} else {
					beads, _ = listBeadsWithFilter(statusFilter)
					refreshTasksWindowWithBeads(w, beads)
				}
			case "Open":
				statusFilter = "open"
				beads, _ = listBeadsWithFilter(statusFilter)
				refreshTasksWindowWithBeads(w, beads)
			case "InProgress":
				statusFilter = "in_progress"
				beads, _ = listBeadsWithFilter(statusFilter)
				refreshTasksWindowWithBeads(w, beads)
			case "Closed":
				statusFilter = "closed"
				beads, _ = listBeadsWithFilter(statusFilter)
				refreshTasksWindowWithBeads(w, beads)
			default:
				w.WriteEvent(e)
			}
		case 'l', 'L':
			text := strings.TrimSpace(string(e.Text))
			if idx := parseIndex(text); idx > 0 && idx <= len(beads) {
				if err := openViewBeadWindow(beads[idx-1].ID); err != nil {
					fmt.Fprintf(os.Stderr, "Error opening bead window: %v\n", err)
				}
			} else {
				w.WriteEvent(e)
			}
		default:
			w.WriteEvent(e)
		}
	}
}

func refreshTasksWindow(w *acme.Win) {
	beads, _ := listBeadsWithFilter("")
	refreshTasksWindowWithBeads(w, beads)
}

func refreshTasksWindowWithBeads(w *acme.Win, beads []Bead) {
	var buf strings.Builder
	buf.WriteString("[Open] [InProgress] [Closed]\n\n")
	buf.WriteString(fmt.Sprintf("%-4s %-12s %-12s %-4s %-8s %s\n", "#", "ID", "Status", "Blk", "Assignee", "Title"))
	buf.WriteString(fmt.Sprintf("%-4s %-12s %-12s %-4s %-8s %s\n", "----", "------------", "------------", "----", "--------", strings.Repeat("-", 50)))

	for i, b := range beads {
		assignee := b.Assignee
		if assignee == "" {
			assignee = "-"
		}
		blk := "-"
		if len(b.Blockers) > 0 {
			blk = fmt.Sprintf("%d", len(b.Blockers))
		}
		buf.WriteString(fmt.Sprintf("%-4d %-12s %-12s %-4s %-8s %s\n", i+1, b.ID, b.Status, blk, assignee, b.Title))
	}

	w.Addr(",")
	w.Write("data", []byte(buf.String()))
	w.Ctl("clean")
	w.Addr("0")
	w.Ctl("dot=addr")
	w.Ctl("show")
}

func listBeadsWithFilter(statusFilter string) ([]Bead, error) {
	if !isConnected() {
		return nil, fmt.Errorf("not connected to anvilsrv")
	}

	data, err := readFile("beads/list")
	if err != nil {
		return nil, err
	}

	var beads []Bead
	if err := json.Unmarshal(data, &beads); err != nil {
		return nil, err
	}

	// Apply status filter
	if statusFilter == "" {
		// Default: filter out closed beads
		var openBeads []Bead
		for _, b := range beads {
			if b.Status != "closed" {
				openBeads = append(openBeads, b)
			}
		}
		return openBeads, nil
	}

	// Filter by specific status
	var filtered []Bead
	for _, b := range beads {
		if b.Status == statusFilter {
			filtered = append(filtered, b)
		}
	}
	return filtered, nil
}

func openNewBeadWindow(parentID string) error {
	w, err := acme.New()
	if err != nil {
		return err
	}

	if parentID != "" {
		w.Name(fmt.Sprintf("/AnviLLM/Tasks/%s/+new", parentID))
	} else {
		w.Name("/AnviLLM/Tasks/+new")
	}
	w.Write("tag", []byte("Put "))

	template := `---
title:
blockers:
---
`
	w.Write("body", []byte(template))
	w.Ctl("clean")

	go handleNewBeadWindow(w, parentID)
	return nil
}

func handleNewBeadWindow(w *acme.Win, parentID string) {
	defer w.CloseFiles()

	for e := range w.EventChan() {
		if (e.C2 == 'x' || e.C2 == 'X') && string(e.Text) == "Put" {
			body, err := w.ReadAll("body")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reading body: %v\n", err)
				continue
			}
			if err := createBeadFromMarkdown(string(body), parentID); err != nil {
				fmt.Fprintf(os.Stderr, "Error creating bead: %v\n", err)
				continue
			}
			w.Ctl("delete")
			return
		} else {
			w.WriteEvent(e)
		}
	}
}

func createBeadFromMarkdown(content, parentID string) error {
	if !isConnected() {
		return fmt.Errorf("not connected to anvilsrv")
	}

	// Parse frontmatter
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---") {
		return fmt.Errorf("missing frontmatter")
	}

	parts := strings.SplitN(content[3:], "---", 2)
	if len(parts) < 1 {
		return fmt.Errorf("invalid frontmatter")
	}

	frontmatter := strings.TrimSpace(parts[0])
	description := ""
	if len(parts) > 1 {
		description = strings.TrimSpace(parts[1])
	}

	// Parse YAML frontmatter (simple key: value parsing)
	var title, blockers string
	for _, line := range strings.Split(frontmatter, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "title:") {
			title = strings.TrimSpace(strings.TrimPrefix(line, "title:"))
		} else if strings.HasPrefix(line, "blockers:") {
			blockers = strings.TrimSpace(strings.TrimPrefix(line, "blockers:"))
		}
	}

	if title == "" {
		return fmt.Errorf("title is required")
	}

	// Build command: new 'title' 'description' [parent-id]
	cmd := fmt.Sprintf("new '%s' '%s'", title, description)
	if parentID != "" {
		cmd = fmt.Sprintf("%s %s", cmd, parentID)
	}

	if err := writeFile("beads/ctl", []byte(cmd)); err != nil {
		return err
	}

	// Add blockers if specified
	if blockers != "" {
		// Need to find the newly created bead ID
		beads, err := listBeadsWithFilter("")
		if err != nil {
			return nil // bead created, deps failed
		}
		var newID string
		for _, b := range beads {
			if b.Title == title {
				newID = b.ID
				break
			}
		}
		if newID != "" {
			for _, blockerID := range strings.Split(blockers, ",") {
				blockerID = strings.TrimSpace(blockerID)
				if blockerID != "" {
					// blockerID blocks newID
					addBlocksDep(blockerID, newID)
				}
			}
		}
	}

	return nil
}

func openViewBeadWindow(beadID string) error {
	w, err := acme.New()
	if err != nil {
		return err
	}

	w.Name(fmt.Sprintf("/AnviLLM/Tasks/%s", beadID))
	w.Write("tag", []byte("Get Put New Blocks "))

	go handleViewBeadWindow(w, beadID)
	return nil
}

func handleViewBeadWindow(w *acme.Win, beadID string) {
	defer w.CloseFiles()

	// Track original blockers for diff on Put
	bead, _ := getBead(beadID)
	var origBlockers []string
	if bead != nil {
		origBlockers = bead.Blockers
	}

	refreshViewBeadWindow(w, beadID)

	for e := range w.EventChan() {
		switch e.C2 {
		case 'x', 'X':
			cmd := string(e.Text)
			arg := strings.TrimSpace(string(e.Arg))
			switch cmd {
			case "Get":
				bead, _ = getBead(beadID)
				if bead != nil {
					origBlockers = bead.Blockers
				}
				refreshViewBeadWindow(w, beadID)
			case "Put":
				body, err := w.ReadAll("body")
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error reading body: %v\n", err)
					continue
				}
				if err := updateBead(beadID, string(body), origBlockers); err != nil {
					fmt.Fprintf(os.Stderr, "Error updating bead: %v\n", err)
				} else {
					bead, _ = getBead(beadID)
					if bead != nil {
						origBlockers = bead.Blockers
					}
					w.Ctl("clean")
				}
			case "New":
				if err := openNewBeadWindow(beadID); err != nil {
					fmt.Fprintf(os.Stderr, "Error opening new bead window: %v\n", err)
				}
			case "Blocks":
				if arg == "" {
					fmt.Fprintf(os.Stderr, "Usage: select bead ID, then Blocks\n")
				} else if err := addBlocksDep(beadID, arg); err != nil {
					fmt.Fprintf(os.Stderr, "Error adding blocks dep: %v\n", err)
				}
			default:
				w.WriteEvent(e)
			}
		default:
			w.WriteEvent(e)
		}
	}
}

func refreshViewBeadWindow(w *acme.Win, beadID string) {
	bead, err := getBead(beadID)
	if err != nil {
		w.Addr(",")
		w.Write("data", []byte(fmt.Sprintf("Error reading bead: %v\n", err)))
		w.Ctl("clean")
		return
	}

	var buf strings.Builder
	buf.WriteString("---\n")
	buf.WriteString(fmt.Sprintf("id: %s\n", bead.ID))
	buf.WriteString(fmt.Sprintf("title: %s\n", bead.Title))
	buf.WriteString(fmt.Sprintf("status: %s\n", bead.Status))
	if bead.Assignee != "" {
		buf.WriteString(fmt.Sprintf("assignee: %s\n", bead.Assignee))
	}
	if bead.Priority != 0 {
		buf.WriteString(fmt.Sprintf("priority: %d\n", bead.Priority))
	}
	buf.WriteString(fmt.Sprintf("blockers: %s\n", strings.Join(bead.Blockers, ", ")))
	buf.WriteString("---\n")
	if bead.Description != "" {
		desc, _ := strconv.Unquote(`"` + bead.Description + `"`)
		if desc == "" {
			desc = bead.Description
		}
		buf.WriteString(desc)
	}

	w.Addr(",")
	w.Write("data", []byte(buf.String()))
	w.Ctl("clean")
}

func getBead(beadID string) (*Bead, error) {
	if !isConnected() {
		return nil, fmt.Errorf("not connected to anvilsrv")
	}

	data, err := readFile(fmt.Sprintf("beads/%s/json", beadID))
	if err != nil {
		return nil, err
	}

	var bead Bead
	if err := json.Unmarshal(data, &bead); err != nil {
		return nil, err
	}

	return &bead, nil
}

func addBlocksDep(blockerID, blockedID string) error {
	if !isConnected() {
		return fmt.Errorf("not connected to anvilsrv")
	}
	// dep <child> <parent> means parent blocks child
	// So "blockerID blocks blockedID" means blockedID depends on blockerID
	cmd := fmt.Sprintf("dep %s %s", blockedID, blockerID)
	return writeFile("beads/ctl", []byte(cmd))
}

func removeBlocksDep(beadID, blockerID string) error {
	if !isConnected() {
		return fmt.Errorf("not connected to anvilsrv")
	}
	cmd := fmt.Sprintf("undep %s %s", beadID, blockerID)
	return writeFile("beads/ctl", []byte(cmd))
}

func deleteBead(beadID string) error {
	if !isConnected() {
		return fmt.Errorf("not connected to anvilsrv")
	}
	cmd := fmt.Sprintf("delete %s", beadID)
	return writeFile("beads/ctl", []byte(cmd))
}

func updateBead(beadID, content string, origBlockers []string) error {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---") {
		return fmt.Errorf("missing frontmatter")
	}

	parts := strings.SplitN(content[3:], "---", 2)
	if len(parts) < 1 {
		return fmt.Errorf("invalid frontmatter")
	}

	frontmatter := strings.TrimSpace(parts[0])
	description := ""
	if len(parts) > 1 {
		description = strings.TrimSpace(parts[1])
	}

	// Parse frontmatter
	var title string
	var newBlockers []string
	for _, line := range strings.Split(frontmatter, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "title:") {
			title = strings.TrimSpace(strings.TrimPrefix(line, "title:"))
		} else if strings.HasPrefix(line, "blockers:") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "blockers:"))
			if val != "" {
				for _, b := range strings.Split(val, ",") {
					b = strings.TrimSpace(b)
					if b != "" {
						newBlockers = append(newBlockers, b)
					}
				}
			}
		}
	}

	// Update title and description
	if title != "" {
		cmd := fmt.Sprintf("update %s title '%s'", beadID, title)
		writeFile("beads/ctl", []byte(cmd))
	}
	if description != "" {
		cmd := fmt.Sprintf("update %s description '%s'", beadID, description)
		writeFile("beads/ctl", []byte(cmd))
	}

	// Update blockers
	origSet := make(map[string]bool)
	for _, b := range origBlockers {
		origSet[b] = true
	}
	newSet := make(map[string]bool)
	for _, b := range newBlockers {
		newSet[b] = true
	}

	for _, b := range newBlockers {
		if !origSet[b] {
			addBlocksDep(b, beadID)
		}
	}
	for _, b := range origBlockers {
		if !newSet[b] {
			removeBlocksDep(beadID, b)
		}
	}

	return nil
}
