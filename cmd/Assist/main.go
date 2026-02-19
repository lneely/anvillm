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
	w.Write("tag", []byte("Get Attach Stop Restart Kill Alias Context Daemon Inbox Archive "))
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
				if err := openInboxWindow(); err != nil {
					fmt.Fprintf(os.Stderr, "Error opening inbox window: %v\n", err)
				}
			case "Archive":
				if err := openArchiveWindow(); err != nil {
					fmt.Fprintf(os.Stderr, "Error opening archive window: %v\n", err)
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

func sendPrompt(id, prompt string) error {
	if !isConnected() {
		return fmt.Errorf("not connected to anvilsrv")
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
	buf.WriteString("Backends: [Kiro] [Claude]\n\n")

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
	ID      string `json:"id"`
	From    string `json:"from"`
	To      string `json:"to"`
	Type    string `json:"type"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

func openInboxWindow() error {
	w, err := acme.New()
	if err != nil {
		return err
	}

	w.Name("/AnviLLM/inbox")
	w.Write("tag", []byte("Get "))

	go handleInboxWindow(w)
	return nil
}

func handleInboxWindow(w *acme.Win) {
	defer w.CloseFiles()

	refreshInboxWindow(w)

	for e := range w.EventChan() {
		switch e.C2 {
		case 'x', 'X':
			cmd := string(e.Text)
			if cmd == "Get" {
				refreshInboxWindow(w)
			} else {
				w.WriteEvent(e)
			}
		case 'l', 'L':
			text := strings.TrimSpace(string(e.Text))
			if isUUID(text) {
				openMessageWindowByID(text, "user/inbox")
			} else {
				w.WriteEvent(e)
			}
		default:
			w.WriteEvent(e)
		}
	}
}

func refreshInboxWindow(w *acme.Win) {
	messages, _, err := listInboxMessages()
	if err != nil {
		w.Addr(",")
		w.Write("data", []byte(fmt.Sprintf("Error reading inbox: %v\n", err)))
		w.Ctl("clean")
		return
	}

	var buf strings.Builder
	buf.WriteString("User Inbox\n")
	buf.WriteString(strings.Repeat("=", 120) + "\n\n")
	buf.WriteString(fmt.Sprintf("%-38s %-12s %-18s %s\n", "ID", "From", "Type", "Subject"))
	buf.WriteString(fmt.Sprintf("%-38s %-12s %-18s %s\n", strings.Repeat("-", 36), "------------", "------------------", strings.Repeat("-", 50)))

	for _, msg := range messages {
		from := msg.From
		if from == "" {
			from = "-"
		}
		buf.WriteString(fmt.Sprintf("%-38s %-12s %-18s %s\n", msg.ID, from, msg.Type, msg.Subject))
	}

	w.Addr(",")
	w.Write("data", []byte(buf.String()))
	w.Ctl("clean")
}

func openArchiveWindow() error {
	w, err := acme.New()
	if err != nil {
		return err
	}

	w.Name("/AnviLLM/archive")
	w.Write("tag", []byte("Get "))

	go handleArchiveWindow(w)
	return nil
}

func handleArchiveWindow(w *acme.Win) {
	defer w.CloseFiles()

	refreshArchiveWindow(w)

	for e := range w.EventChan() {
		switch e.C2 {
		case 'x', 'X':
			if string(e.Text) == "Get" {
				refreshArchiveWindow(w)
			} else {
				w.WriteEvent(e)
			}
		case 'l', 'L':
			text := strings.TrimSpace(string(e.Text))
			if isUUID(text) {
				openMessageWindowByID(text, "user/completed")
			} else {
				w.WriteEvent(e)
			}
		default:
			w.WriteEvent(e)
		}
	}
}

func refreshArchiveWindow(w *acme.Win) {
	messages, _, err := listArchiveMessages()
	if err != nil {
		w.Addr(",")
		w.Write("data", []byte(fmt.Sprintf("Error reading archive: %v\n", err)))
		w.Ctl("clean")
		return
	}

	var buf strings.Builder
	buf.WriteString("User Archive\n")
	buf.WriteString(strings.Repeat("=", 120) + "\n\n")
	buf.WriteString(fmt.Sprintf("%-38s %-12s %-18s %s\n", "ID", "From", "Type", "Subject"))
	buf.WriteString(fmt.Sprintf("%-38s %-12s %-18s %s\n", strings.Repeat("-", 36), "------------", "------------------", strings.Repeat("-", 50)))

	for _, msg := range messages {
		from := msg.From
		if from == "" {
			from = "-"
		}
		buf.WriteString(fmt.Sprintf("%-38s %-12s %-18s %s\n", msg.ID, from, msg.Type, msg.Subject))
	}

	w.Addr(",")
	w.Write("data", []byte(buf.String()))
	w.Ctl("clean")
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
			refreshInboxWindow(w)
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
