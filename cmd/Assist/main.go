// Assist - Acme interface for AnviLLM
package main

import (
	"anvillm/internal/logging"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
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
	"go.uber.org/zap"
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
	Role    string
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
	if err := logging.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logging: %v\n", err)
		os.Exit(1)
	}
	defer logging.Logger().Sync()

	flag.Parse()

	// Connect to anvilsrv via 9P, auto-starting if needed
	var err error
	fs, err = connectToServer()
	if err != nil {
		// Try to start anvilsrv automatically
		logging.Logger().Info("anvilsrv not running, attempting to start")
		startCmd := exec.Command("anvilsrv", "start")
		if err := startCmd.Run(); err != nil {
			logging.Logger().Error("failed to start anvilsrv", zap.Error(err))
			logging.Logger().Info("continuing without daemon connection")
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
				logging.Logger().Error("failed to connect to anvilsrv after starting", zap.Error(err))
				logging.Logger().Info("continuing without daemon connection")
			}
		}
	}
	if fs != nil {
		defer fs.Close()
	}

	w, err := acme.New()
	if err != nil {
		logging.Logger().Fatal("failed to create acme window", zap.Error(err))
	}
	defer w.CloseFiles()

	w.Name(windowName)
	w.Write("tag", []byte("Get Put Attach Stop Restart Kill Alias Context Daemon Recover Inbox Archive Tasks "))
	refreshList(w)
	w.Ctl("clean")

	// Event loop
	for e := range w.EventChan() {
		switch e.C2 {
		case 'x', 'X':
			cmd := string(e.Text)
			arg := strings.TrimSpace(string(e.Arg))

			// Handle Put: apply inline session edits from body
			if cmd == "Put" {
				body, err := w.ReadAll("body")
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error reading body: %v\n", err)
					continue
				}
				edits := parseSessionEdits(string(body))
				applySessionEdits(w, edits)
				continue
			}

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
			case "Recover":
				if err := recoverSessions(w); err != nil {
					fmt.Fprintf(os.Stderr, "Error recovering sessions: %v\n", err)
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
	return client.MountService("anvillm")
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

func getMountForSession(sessionID string) string {
	sess, err := getSession(sessionID)
	if err != nil || sess.Cwd == "" {
		return ""
	}
	data, err := readFile("beads/mtab")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.Split(line, "\t")
		if len(parts) == 2 {
			mountName, mountCwd := parts[0], parts[1]
			if sess.Cwd == mountCwd || strings.HasPrefix(sess.Cwd, mountCwd+"/") {
				return mountName
			}
		}
	}
	return ""
}

func sendPrompt(id, prompt string) error {
	mount := ""
	trimmedPrompt := strings.TrimSpace(prompt)
	if isBeadID(trimmedPrompt) {
		mount = getMountForSession(id)
	}
	return sendPromptWithMount(id, prompt, mount)
}

func sendPromptWithMount(id, prompt, mount string) error {
	if !isConnected() {
		return fmt.Errorf("not connected to anvilsrv")
	}

	// Check if prompt is a bead ID and construct execution prompt
	trimmedPrompt := strings.TrimSpace(prompt)
	if isBeadID(trimmedPrompt) {
		if mount != "" {
			prompt = fmt.Sprintf("Load the beads skill, and work on bead %s, mount=%s.", trimmedPrompt, mount)
		} else {
			prompt = fmt.Sprintf("Load the beads skill, and work on bead %s.", trimmedPrompt)
		}
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

		// Parse: id backend state alias model cwd
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}

		sess := &SessionInfo{
			ID:      fields[0],
			Backend: fields[1],
			State:   fields[2],
			Alias:   fields[3],
			Role:    fields[4],
			Cwd:     strings.Join(fields[5:], " "),
		}
		if sess.Alias == "-" {
			sess.Alias = ""
		}
		if sess.Role == "-" {
			sess.Role = ""
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
		w.Addr("0")
		w.Ctl("dot=addr")
		w.Ctl("show")
		return
	}

	sessions, err := listSessions()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to list sessions: %v\n", err)
		buf.WriteString("Error listing sessions: " + err.Error() + "\n")
		w.Addr(",")
		w.Write("data", []byte(buf.String()))
		w.Ctl("clean")
		w.Addr("0")
		w.Ctl("dot=addr")
		w.Ctl("show")
		return
	}

	buf.WriteString(fmt.Sprintf("%-8s %-10s %-16s %-9s %-16s %s\n", "ID", "Backend", "Role", "State", "Alias", "Cwd"))
	buf.WriteString(fmt.Sprintf("%-8s %-10s %-16s %-9s %-16s %s\n", "--------", "----------", "----------------", "---------", "----------------", strings.Repeat("-", 40)))

	for _, sess := range sessions {
		alias := sess.Alias
		if alias == "" {
			alias = "-"
		}
		role := sess.Role
		if role == "" {
			role = "-"
		}
		buf.WriteString(fmt.Sprintf("%-8s %-10s %-16s %-9s %-16s %s\n", sess.ID, sess.Backend, role, sess.State, alias, sess.Cwd))
	}

	w.Addr(",")
	w.Write("data", []byte(buf.String()))
	w.Ctl("clean")
	w.Addr("0")
	w.Ctl("dot=addr")
	w.Ctl("show")
}

func recoverSessions(w *acme.Win) error {
	if !isConnected() {
		return fmt.Errorf("not connected to anvilsrv")
	}
	if err := writeFile("ctl", []byte("recover")); err != nil {
		return err
	}
	refreshList(w)
	return nil
}

// sessionEdit represents a single inline action parsed from the sessions window body.
type sessionEdit struct {
	action  string // "kill", "stop", "alias", "start"
	id      string // session ID (for kill/stop/alias)
	backend string // backend name (for start)
	path    string // working directory (for start)
	alias   string // alias name (for alias/start)
}

// parseSessionEdits parses inline action annotations from the sessions window body.
// Recognized prefixes (borrowed from the parseEdits pattern in permissions.go):
//
//	- <id>                    Kill the session with that ID
//	~ <id>                    Stop the session (graceful)
//	@ <id> <alias>            Set alias on the session
//	+ <backend> <path> [alias]  Start a new session (backends: claude, kiro-cli, ollama)
func parseSessionEdits(content string) []sessionEdit {
	var edits []sessionEdit
	validBackends := map[string]bool{
		"claude":   true,
		"kiro-cli": true,
		"ollama":   true,
	}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if len(line) == 0 || strings.HasPrefix(line, "#") {
			continue
		}
		switch {
		case strings.HasPrefix(line, "- "):
			parts := strings.Fields(line[2:])
			if len(parts) >= 1 && sessionIDRegex.MatchString(parts[0]) {
				edits = append(edits, sessionEdit{action: "kill", id: parts[0]})
			}
		case strings.HasPrefix(line, "~ "):
			parts := strings.Fields(line[2:])
			if len(parts) >= 1 && sessionIDRegex.MatchString(parts[0]) {
				edits = append(edits, sessionEdit{action: "stop", id: parts[0]})
			}
		case strings.HasPrefix(line, "@ "):
			parts := strings.Fields(line[2:])
			if len(parts) >= 2 {
				id := parts[0]
				alias := parts[1]
				if sessionIDRegex.MatchString(id) && aliasRegex.MatchString(alias) {
					edits = append(edits, sessionEdit{action: "alias", id: id, alias: alias})
				}
			}
		case strings.HasPrefix(line, "+ "):
			parts := strings.Fields(line[2:])
			if len(parts) >= 2 {
				backend := parts[0]
				path := parts[1]
				if validBackends[backend] {
					alias := ""
					if len(parts) >= 3 && aliasRegex.MatchString(parts[2]) {
						alias = parts[2]
					}
					edits = append(edits, sessionEdit{action: "start", backend: backend, path: path, alias: alias})
				}
			}
		}
	}
	return edits
}

// applySessionEdits applies a batch of session edits atomically, then refreshes the list.
// For newly started sessions that need an alias, it retries asynchronously until the
// session appears, then sets the alias.
func applySessionEdits(w *acme.Win, edits []sessionEdit) {
	if len(edits) == 0 {
		refreshList(w)
		return
	}

	// Snapshot existing session IDs before any starts so we can identify new ones.
	existingSessions, _ := listSessions()
	existingIDs := make(map[string]bool)
	for _, s := range existingSessions {
		existingIDs[s.ID] = true
	}

	// pendingAliases holds an alias string per start-action that requested one.
	// "" means no alias wanted for that slot.
	var pendingAliases []string
	var errs []string

	for _, edit := range edits {
		switch edit.action {
		case "kill":
			if err := controlSession(edit.id, "kill"); err != nil {
				errs = append(errs, fmt.Sprintf("kill %s: %v", edit.id, err))
			}
		case "stop":
			if err := controlSession(edit.id, "stop"); err != nil {
				errs = append(errs, fmt.Sprintf("stop %s: %v", edit.id, err))
			}
		case "alias":
			if err := setAlias(edit.id, edit.alias); err != nil {
				errs = append(errs, fmt.Sprintf("alias %s %s: %v", edit.id, edit.alias, err))
			}
		case "start":
			if err := createSession(edit.backend, edit.path); err != nil {
				errs = append(errs, fmt.Sprintf("start %s: %v", edit.backend, err))
			} else {
				pendingAliases = append(pendingAliases, edit.alias)
			}
		}
	}

	if len(errs) > 0 {
		fmt.Fprintf(os.Stderr, "Session edit errors:\n%s\n", strings.Join(errs, "\n"))
	}

	// If any start-actions requested aliases, resolve them asynchronously: poll
	// until the new sessions appear (up to ~10 s), assign aliases in FIFO order,
	// then do a final list refresh.
	if len(pendingAliases) > 0 {
		go func() {
			assigned := make([]bool, len(pendingAliases))
			for attempt := 0; attempt < 20; attempt++ {
				time.Sleep(500 * time.Millisecond)
				sessions, err := listSessions()
				if err != nil {
					continue
				}
				// Collect new session IDs in appearance order.
				var newSessions []*SessionInfo
				for _, s := range sessions {
					if !existingIDs[s.ID] {
						newSessions = append(newSessions, s)
					}
				}
				// Match pending aliases to new sessions in FIFO order.
				newIdx := 0
				for i, alias := range pendingAliases {
					if assigned[i] {
						newIdx++ // skip slots already resolved
						continue
					}
					if alias == "" {
						assigned[i] = true
						continue
					}
					if newIdx < len(newSessions) {
						if setAlias(newSessions[newIdx].ID, alias) == nil {
							assigned[i] = true
						}
						newIdx++
					}
				}
				// Check if all slots are resolved.
				allDone := true
				for _, done := range assigned {
					if !done {
						allDone = false
						break
					}
				}
				if allDone {
					break
				}
			}
			refreshList(w)
		}()
	}

	refreshList(w)
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
	w.Write("tag", []byte("Send Compact Clear Resume "))
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
		if e.C2 == 'x' || e.C2 == 'X' {
			switch cmd {
			case "Send":
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
			case "Compact":
				if err := controlSession(sess.ID, "compact"); err != nil {
					fmt.Fprintf(os.Stderr, "Failed to compact: %v\n", err)
				}
			case "Clear":
				if err := controlSession(sess.ID, "clear"); err != nil {
					fmt.Fprintf(os.Stderr, "Failed to clear: %v\n", err)
				}
			case "Resume":
				if err := controlSession(sess.ID, "resume"); err != nil {
					fmt.Fprintf(os.Stderr, "Failed to resume: %v\n", err)
				}
			default:
				w.WriteEvent(e)
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
					logging.Logger().Info("context updated", zap.String("session", sess.ID))
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
		sockPath := filepath.Join(ns, "anvillm")
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
	w.Addr("0")
	w.Ctl("dot=addr")
	w.Ctl("show")
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
	w.Write("tag", []byte("Get Put "))

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
			switch string(e.Text) {
			case "Get":
				refreshMailboxWindow(w, folder, "Inbox")
			case "Put":
				messages, _, err := listMessages(folder)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error listing messages: %v\n", err)
					continue
				}
				body, err := w.ReadAll("body")
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error reading body: %v\n", err)
					continue
				}
				edits := parseMailEdits(string(body), messages)
				var errs []string
				for _, edit := range edits {
					if err := archiveMessage(edit.msgID); err != nil {
						errs = append(errs, fmt.Sprintf("archive %s: %v", edit.msgID, err))
					}
				}
				if len(errs) > 0 {
					fmt.Fprintf(os.Stderr, "Put errors: %s\n", strings.Join(errs, "; "))
				}
				refreshMailboxWindow(w, folder, "Inbox")
			default:
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
	buf.WriteString(fmt.Sprintf("%-10s %-20s %-12s %-1s %-18s %s\n", "ID", "Date", "From", "!", "Type", "Subject"))
	buf.WriteString(fmt.Sprintf("%-10s %-20s %-12s %-1s %-18s %s\n", "----------", "--------------------", "------------", "-", "------------------", strings.Repeat("-", 40)))

	for _, msg := range messages {
		from := msg.From
		if from == "" {
			from = "-"
		}
		shortID := shortUUID(msg.ID)
		dateStr := formatTimestamp(msg.Timestamp)
		// Mark messages that require user approval or review action
		flag := " "
		if msg.Type == "APPROVAL_REQUEST" || msg.Type == "REVIEW_REQUEST" {
			flag = "!"
		}
		buf.WriteString(fmt.Sprintf("%-10s %-20s %-12s %-1s %-18s %s\n", shortID, dateStr, from, flag, msg.Type, msg.Subject))
	}

	w.Addr(",")
	w.Write("data", []byte(buf.String()))
	w.Ctl("clean")
	w.Addr("0")
	w.Ctl("dot=addr")
	w.Ctl("show")
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
	w.Write("tag", []byte("Get Put "))

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
			switch string(e.Text) {
			case "Get":
				refreshMailboxWindow(w, folder, "Archive")
			case "Put":
				messages, _, err := listMessages(folder)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error listing messages: %v\n", err)
					continue
				}
				body, err := w.ReadAll("body")
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error reading body: %v\n", err)
					continue
				}
				edits := parseMailEdits(string(body), messages)
				var errs []string
				for _, edit := range edits {
					if err := deleteArchivedMessage(edit.msgID); err != nil {
						errs = append(errs, fmt.Sprintf("delete %s: %v", edit.msgID, err))
					}
				}
				if len(errs) > 0 {
					fmt.Fprintf(os.Stderr, "Put errors: %s\n", strings.Join(errs, "; "))
				}
				refreshMailboxWindow(w, folder, "Archive")
			default:
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

	// Add Approve/Reject buttons for messages requiring user action
	tagStr := "Reply Archive "
	if msg.Type == "APPROVAL_REQUEST" || msg.Type == "REVIEW_REQUEST" {
		tagStr = "Approve Reject Reply Archive "
	}
	w.Write("tag", []byte(tagStr))

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
		cmd := string(e.Text)
		if e.C2 == 'x' || e.C2 == 'X' {
			switch cmd {
			case "Reply":
				openReplyWindow(msg)
			case "Approve":
				openApproveWindow(msg)
			case "Reject":
				openRejectWindow(msg)
			case "Archive":
				if err := archiveMessage(msg.ID); err != nil {
					w.Fprintf("errors", "Archive failed: %v\n", err)
				} else {
					refreshInboxWindowByName()
					w.Ctl("delete")
				}
			default:
				w.WriteEvent(e)
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

	// Pass "" for originalMsgID — plain replies do not auto-archive
	go handleReplyWindow(w, originalMsg.From, replyType, replySubject, "")
	return nil
}

// openApproveWindow opens a reply window pre-filled with "Approved." for one-click approval.
// On Send the original message is automatically archived.
func openApproveWindow(originalMsg *Message) error {
	w, err := acme.New()
	if err != nil {
		return err
	}

	replyType := getReplyType(originalMsg.Type)
	replySubject := fmt.Sprintf("Re: %s", originalMsg.Subject)

	w.Name(fmt.Sprintf("/AnviLLM/reply/%s", originalMsg.From))
	w.Write("tag", []byte("Send "))
	w.Write("body", []byte("Approved."))
	w.Ctl("clean")

	go handleReplyWindow(w, originalMsg.From, replyType, replySubject, originalMsg.ID)
	return nil
}

// openRejectWindow opens a reply window pre-filled with a rejection template,
// prompting the user to supply a reason before sending.
// On Send the original message is automatically archived.
func openRejectWindow(originalMsg *Message) error {
	w, err := acme.New()
	if err != nil {
		return err
	}

	replyType := getReplyType(originalMsg.Type)
	replySubject := fmt.Sprintf("Re: %s", originalMsg.Subject)

	w.Name(fmt.Sprintf("/AnviLLM/reply/%s", originalMsg.From))
	w.Write("tag", []byte("Send "))
	w.Write("body", []byte("Rejected.\n\nReason: "))
	w.Ctl("clean")

	go handleReplyWindow(w, originalMsg.From, replyType, replySubject, originalMsg.ID)
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

// deleteArchivedMessage permanently removes a message from the completed/archive folder.
func deleteArchivedMessage(msgID string) error {
	if !isConnected() {
		return fmt.Errorf("not connected to anvilsrv")
	}

	fid, err := fs.Open("user/ctl", plan9.OWRITE)
	if err != nil {
		return err
	}
	defer fid.Close()

	ctlMsg := fmt.Sprintf("delete %s", msgID)
	_, err = fid.Write([]byte(ctlMsg))
	return err
}

// mailEdit represents a single inline archive/delete action from a mailbox window body.
type mailEdit struct {
	msgID string // full message ID (expanded from short-ID prefix)
}

// parseMailEdits parses inline action annotations from a mailbox window body.
// Lines starting with "- <shortID>" mark messages for bulk action (archive or delete).
func parseMailEdits(content string, messages []Message) []mailEdit {
	var edits []mailEdit
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- ") {
			continue
		}
		parts := strings.Fields(line[2:])
		if len(parts) >= 1 && isHexString(parts[0]) {
			fullID := expandUUID(parts[0], messages)
			edits = append(edits, mailEdit{msgID: fullID})
		}
	}
	return edits
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

func handleReplyWindow(w *acme.Win, to, msgType, subject, originalMsgID string) {
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
				// Auto-archive the original message when replying from approve/reject
				if originalMsgID != "" {
					if err := archiveMessage(originalMsgID); err != nil {
						fmt.Fprintf(os.Stderr, "Failed to archive original message: %v\n", err)
					} else {
						refreshInboxWindowByName()
					}
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
	ID               string   `json:"id"`
	Title            string   `json:"title"`
	Description      string   `json:"description"`
	Status           string   `json:"status"`
	Assignee         string   `json:"assignee"`
	Priority         int      `json:"priority"`
	Blockers         []string `json:"blockers,omitempty"`
	Labels           []string `json:"labels,omitempty"`
	CloseReason      string   `json:"close_reason,omitempty"`
}

func (b *Bead) HasLabel(label string) bool {
	for _, l := range b.Labels {
		if l == label {
			return true
		}
	}
	return false
}

// beadEdit represents a single inline action parsed from the tasks window body.
type beadEdit struct {
	action string // "delete", "claim", "unclaim", "complete", "fail", "create"
	beadID string // bead ID (for existing bead actions)
	title  string // title (for create action)
}

// parseBeadEdits parses inline action annotations from the tasks window body.
// Recognized prefixes (by 1-based index shown in the list):
//
//	- [idx]   Delete bead at that index
//	~ [idx]   Claim bead at that index (assign to user)
//	^ [idx]   Unclaim bead: clear assignee, reset status to open
//	x [idx]   Close/complete bead at that index
//	f [idx]   Fail bead at that index with reason 'user-closed'
//	+ [title] Create new bead with that title
func parseBeadEdits(content string, beads []Bead) []beadEdit {
	var edits []beadEdit
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		switch {
		case strings.HasPrefix(line, "- "):
			rest := strings.TrimSpace(line[2:])
			if idx := parseIndex(rest); idx > 0 && idx <= len(beads) {
				edits = append(edits, beadEdit{action: "delete", beadID: beads[idx-1].ID})
			}
		case strings.HasPrefix(line, "~ "):
			rest := strings.TrimSpace(line[2:])
			if idx := parseIndex(rest); idx > 0 && idx <= len(beads) {
				edits = append(edits, beadEdit{action: "claim", beadID: beads[idx-1].ID})
			}
		case strings.HasPrefix(line, "^ "):
			rest := strings.TrimSpace(line[2:])
			if idx := parseIndex(rest); idx > 0 && idx <= len(beads) {
				edits = append(edits, beadEdit{action: "unclaim", beadID: beads[idx-1].ID})
			}
		case strings.HasPrefix(line, "x "):
			rest := strings.TrimSpace(line[2:])
			if idx := parseIndex(rest); idx > 0 && idx <= len(beads) {
				edits = append(edits, beadEdit{action: "complete", beadID: beads[idx-1].ID})
			}
		case strings.HasPrefix(line, "f "):
			rest := strings.TrimSpace(line[2:])
			if idx := parseIndex(rest); idx > 0 && idx <= len(beads) {
				edits = append(edits, beadEdit{action: "fail", beadID: beads[idx-1].ID})
			}
		case strings.HasPrefix(line, "+ "):
			title := strings.TrimSpace(line[2:])
			if title != "" {
				edits = append(edits, beadEdit{action: "create", title: title})
			}
		}
	}
	return edits
}

// applyBeadEdits applies a batch of bead edits.
func applyBeadEdits(w *acme.Win, edits []beadEdit, beads *[]Bead, filter string, mount string) {
	if len(edits) == 0 {
		return
	}
	
	var errs []string
	for _, edit := range edits {
		switch edit.action {
		case "delete":
			if err := deleteBead(edit.beadID, mount); err != nil {
				errs = append(errs, fmt.Sprintf("delete %s: %v", edit.beadID, err))
			}
		case "claim":
			if err := claimBeadAsUser(edit.beadID, mount); err != nil {
				errs = append(errs, fmt.Sprintf("claim %s: %v", edit.beadID, err))
			}
		case "unclaim":
			if err := unclaimBead(edit.beadID, mount); err != nil {
				errs = append(errs, fmt.Sprintf("unclaim %s: %v", edit.beadID, err))
			}
		case "complete":
			if err := completeBead(edit.beadID, mount); err != nil {
				errs = append(errs, fmt.Sprintf("complete %s: %v", edit.beadID, err))
			}
		case "fail":
			if err := failBead(edit.beadID, "user-closed", mount); err != nil {
				errs = append(errs, fmt.Sprintf("fail %s: %v", edit.beadID, err))
			}
		case "create":
			if err := openNewBeadWindowWithTitle(edit.title, mount); err != nil {
				errs = append(errs, fmt.Sprintf("create %q: %v", edit.title, err))
			}
		}
	}
	for _, e := range errs {
		fmt.Fprintf(os.Stderr, "beadEdit error: %s\n", e)
	}
	refreshed, _ := listBeadsWithFilter(filter, mount)
	*beads = refreshed
}

func openTasksWindow() error {
	w, err := acme.New()
	if err != nil {
		return err
	}

	w.Name("/AnviLLM/Tasks")
	w.Write("tag", []byte("Get Put New Remove Init Mount Umount Select "))

	go handleTasksWindow(w)
	return nil
}

func handleTasksWindow(w *acme.Win) {
	defer w.CloseFiles()

	var beads []Bead
	filter := "ready"
	windowMount := ""
	beads, _ = listBeadsWithFilter(filter, windowMount)
	refreshTasksWindowWithBeads(w, beads, windowMount, filter)

	for e := range w.EventChan() {
		switch e.C2 {
		case 'x', 'X':
			cmd := string(e.Text)
			arg := strings.TrimSpace(string(e.Arg))
			switch cmd {
			case "Get":
				beads, _ = listBeadsWithFilter(filter, windowMount)
				refreshTasksWindowWithBeads(w, beads, windowMount, filter)
			case "Put":
				body, err := w.ReadAll("body")
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error reading tasks body: %v\n", err)
					continue
				}
				edits := parseBeadEdits(string(body), beads)
				applyBeadEdits(w, edits, &beads, filter, windowMount)
			case "New":
				if err := openNewBeadWindow("", windowMount); err != nil {
					fmt.Fprintf(os.Stderr, "Error opening new bead window: %v\n", err)
				}
			case "Remove":
				if arg == "" {
					fmt.Fprintf(os.Stderr, "Usage: select bead ID, then Remove\n")
				} else if err := deleteBead(arg, windowMount); err != nil {
					fmt.Fprintf(os.Stderr, "Error deleting bead: %v\n", err)
				} else {
					beads, _ = listBeadsWithFilter(filter, windowMount)
					refreshTasksWindowWithBeads(w, beads, windowMount, filter)
				}
			case "Init":
				prefix := arg
				if prefix == "" {
					prefix = "bd"
				}
				if err := initBeads(prefix, windowMount); err != nil {
					fmt.Fprintf(os.Stderr, "Error initializing beads: %v\n", err)
				} else {
					beads, _ = listBeadsWithFilter(filter, windowMount)
					refreshTasksWindowWithBeads(w, beads, windowMount, filter)
				}
			case "Mount":
				cwd := strings.TrimSpace(arg)
				if cwd == "" {
					fmt.Fprintf(os.Stderr, "Usage: select path, then Mount\n")
				} else {
					if name, err := mountProject(cwd); err != nil {
						fmt.Fprintf(os.Stderr, "Error mounting: %v\n", err)
					} else {
						windowMount = name
						beads, _ = listBeadsWithFilter(filter, windowMount)
						refreshTasksWindowWithBeads(w, beads, windowMount, filter)
					}
				}
			case "Umount":
				name := strings.TrimSpace(arg)
				if name == "" {
					fmt.Fprintf(os.Stderr, "Usage: select mount name, then Umount\n")
				} else if err := umountProject(name); err != nil {
					fmt.Fprintf(os.Stderr, "Error unmounting: %v\n", err)
				} else {
					if windowMount == name {
						data, _ := readFile("beads/mtab")
						lines := strings.Split(strings.TrimSpace(string(data)), "\n")
						found := false
						for _, line := range lines {
							if line != "" {
								parts := strings.Split(line, "\t")
								if len(parts) > 0 {
									windowMount = parts[0]
									found = true
									break
								}
							}
						}
						if !found {
							windowMount = ""
							beads = nil
							refreshTasksWindowWithBeads(w, beads, windowMount, filter)
							continue
						}
					}
					beads, _ = listBeadsWithFilter(filter, windowMount)
					refreshTasksWindowWithBeads(w, beads, windowMount, filter)
				}
			case "Select":
				name := strings.TrimSpace(arg)
				if name == "" {
					data, err := readFile("beads/mtab")
					if err != nil {
						fmt.Fprintf(os.Stderr, "Error reading mtab: %v\n", err)
					} else {
						fmt.Fprintf(os.Stderr, "Current mounts:\n%s\n", string(data))
						fmt.Fprintf(os.Stderr, "Usage: select mount name, then Select\n")
					}
				} else {
					data, err := readFile("beads/mtab")
					if err != nil {
						fmt.Fprintf(os.Stderr, "Error reading mtab: %v\n", err)
					} else {
						found := false
						for _, line := range strings.Split(string(data), "\n") {
							if strings.HasPrefix(line, name+"\t") {
								found = true
								break
							}
						}
						if !found {
							fmt.Fprintf(os.Stderr, "Mount %s not found\n", name)
						} else {
							windowMount = name
							beads, _ = listBeadsWithFilter(filter, windowMount)
							refreshTasksWindowWithBeads(w, beads, windowMount, filter)
						}
					}
				}
			case "All":
				filter = "all"
				beads, _ = listBeadsWithFilter(filter, windowMount)
				refreshTasksWindowWithBeads(w, beads, windowMount, filter)
			case "Ready":
				filter = "ready"
				beads, _ = listBeadsWithFilter(filter, windowMount)
				refreshTasksWindowWithBeads(w, beads, windowMount, filter)
			default:
				w.WriteEvent(e)
			}
		case 'l', 'L':
			text := strings.TrimSpace(string(e.Text))
			if idx := parseIndex(text); idx > 0 && idx <= len(beads) {
				if err := openViewBeadWindow(beads[idx-1].ID, windowMount); err != nil {
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

func refreshTasksWindowWithBeads(w *acme.Win, beads []Bead, mount string, filter string) {
	if mount == "" {
		w.Name("/AnviLLM/Tasks")
	} else {
		w.Name(fmt.Sprintf("/AnviLLM/Tasks [%s]", mount))
	}
	var buf strings.Builder
	
	if mount == "" {
		buf.WriteString("No project mounted.\n\n")
		buf.WriteString("Use Mount to mount a project directory, or Select to choose an existing mount.\n\n")
		buf.WriteString("Example:\n")
		buf.WriteString("  1. Select a directory path, then click Mount\n")
		buf.WriteString("  2. Or click Select to see available mounts\n\n")
	} else {
		buf.WriteString("[All] [Ready]\n\n")
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
	}

	w.Addr(",")
	w.Write("data", []byte(buf.String()))
	w.Ctl("clean")
	w.Addr("0")
	w.Ctl("dot=addr")
	w.Ctl("show")
}

func listBeadsWithFilter(filter string, mount string) ([]Bead, error) {
	if !isConnected() {
		return nil, fmt.Errorf("not connected to anvilsrv")
	}

	if mount == "" {
		return nil, nil
	}

	var endpoint string
	if filter == "ready" {
		endpoint = filepath.Join("beads", mount, "ready")
	} else {
		endpoint = filepath.Join("beads", mount, "list")
	}

	data, err := readFile(endpoint)
	if err != nil {
		return nil, err
	}

	var beads []Bead
	if err := json.Unmarshal(data, &beads); err != nil {
		return nil, err
	}
	return beads, nil
}

func openNewBeadWindow(parentID string, mount string) error {
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

	go handleNewBeadWindow(w, parentID, mount)
	return nil
}

// openNewBeadWindowWithTitle opens a new bead creation window pre-filled with the given title.
func openNewBeadWindowWithTitle(title string, mount string) error {
	w, err := acme.New()
	if err != nil {
		return err
	}

	w.Name("/AnviLLM/Tasks/+new")
	w.Write("tag", []byte("Put "))

	body := fmt.Sprintf("---\ntitle: %s\nblockers:\n---\n", title)
	w.Write("body", []byte(body))
	w.Ctl("clean")

	go handleNewBeadWindow(w, "", mount)
	return nil
}

func handleNewBeadWindow(w *acme.Win, parentID string, mount string) {
	defer w.CloseFiles()

	for e := range w.EventChan() {
		if (e.C2 == 'x' || e.C2 == 'X') && string(e.Text) == "Put" {
			body, err := w.ReadAll("body")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reading body: %v\n", err)
				continue
			}
			if err := createBeadFromMarkdown(string(body), parentID, mount); err != nil {
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

func createBeadFromMarkdown(content, parentID string, mount string) error {
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

	// Build command: new 'title' 'description' [parent-id] [blockers=...]
	cmd := fmt.Sprintf("new '%s' '%s'", title, description)
	if parentID != "" {
		cmd = fmt.Sprintf("%s %s", cmd, parentID)
	}
	if blockers != "" {
		cmd = fmt.Sprintf("%s blockers=%s", cmd, blockers)
	}

	return writeFile(filepath.Join("beads", mount, "ctl"), []byte(cmd))
}

func openViewBeadWindow(beadID string, mount string) error {
	w, err := acme.New()
	if err != nil {
		return err
	}

	w.Name(fmt.Sprintf("/AnviLLM/Tasks/%s", beadID))
	w.Write("tag", []byte("Get Put New Blocks Comments "))

	go handleViewBeadWindow(w, beadID, mount)
	return nil
}

func handleViewBeadWindow(w *acme.Win, beadID string, mount string) {
	defer w.CloseFiles()

	// Track original blockers for diff on Put
	bead, _ := getBead(beadID, mount)
	var origBlockers []string
	if bead != nil {
		origBlockers = bead.Blockers
	}

	refreshViewBeadWindow(w, beadID, mount)

	for e := range w.EventChan() {
		switch e.C2 {
		case 'x', 'X':
			cmd := string(e.Text)
			arg := strings.TrimSpace(string(e.Arg))
			switch cmd {
			case "Get":
				bead, _ = getBead(beadID, mount)
				if bead != nil {
					origBlockers = bead.Blockers
				}
				refreshViewBeadWindow(w, beadID, mount)
			case "Put":
				body, err := w.ReadAll("body")
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error reading body: %v\n", err)
					continue
				}
				if err := updateBead(beadID, string(body), origBlockers, mount); err != nil {
					fmt.Fprintf(os.Stderr, "Error updating bead: %v\n", err)
				} else {
					bead, _ = getBead(beadID, mount)
					if bead != nil {
						origBlockers = bead.Blockers
					}
					w.Ctl("clean")
				}
			case "New":
				if err := openNewBeadWindow(beadID, mount); err != nil {
					fmt.Fprintf(os.Stderr, "Error opening new bead window: %v\n", err)
				}
			case "Blocks":
				if arg == "" {
					fmt.Fprintf(os.Stderr, "Usage: select bead ID, then Blocks\n")
				} else if err := addBlocksDep(beadID, arg, mount); err != nil {
					fmt.Fprintf(os.Stderr, "Error adding blocks dep: %v\n", err)
				}
			case "Comments":
				if err := openCommentsWindow(beadID, mount); err != nil {
					fmt.Fprintf(os.Stderr, "Error opening comments window: %v\n", err)
				}
			default:
				w.WriteEvent(e)
			}
		default:
			w.WriteEvent(e)
		}
	}
}

func refreshViewBeadWindow(w *acme.Win, beadID string, mount string) {
	bead, err := getBead(beadID, mount)
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
	if bead.HasLabel("requires_approval") {
		buf.WriteString("requires_approval: yes\n")
	} else {
		buf.WriteString("requires_approval: no\n")
	}
	if bead.HasLabel("requires_review") {
		buf.WriteString("requires_review: yes\n")
	} else {
		buf.WriteString("requires_review: no\n")
	}
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
	w.Addr("0")
	w.Ctl("dot=addr")
	w.Ctl("show")
}

func openCommentsWindow(beadID string, mount string) error {
	w, err := acme.New()
	if err != nil {
		return err
	}
	w.Name(fmt.Sprintf("/AnviLLM/Tasks/%s/comments", beadID))
	w.Write("tag", []byte("Get "))

	go handleCommentsWindow(w, beadID, mount)
	return nil
}

func handleCommentsWindow(w *acme.Win, beadID string, mount string) {
	defer w.CloseFiles()
	refreshCommentsWindow(w, beadID, mount)

	for e := range w.EventChan() {
		switch e.C2 {
		case 'x', 'X':
			if string(e.Text) == "Get" {
				refreshCommentsWindow(w, beadID, mount)
			} else {
				w.WriteEvent(e)
			}
		default:
			w.WriteEvent(e)
		}
	}
}

func refreshCommentsWindow(w *acme.Win, beadID string, mount string) {
	data, err := readFile(filepath.Join("beads", mount, beadID, "comments"))
	if err != nil {
		w.Addr(",")
		w.Write("data", []byte(fmt.Sprintf("Error reading comments: %v\n", err)))
		w.Ctl("clean")
		return
	}

	var comments []struct {
		Author    string `json:"author"`
		Body      string `json:"body"`
		CreatedAt string `json:"created_at"`
	}
	if err := json.Unmarshal(data, &comments); err != nil {
		w.Addr(",")
		w.Write("data", []byte(fmt.Sprintf("Error parsing comments: %v\n", err)))
		w.Ctl("clean")
		return
	}

	var buf strings.Builder
	if len(comments) == 0 {
		buf.WriteString("No comments.\n")
	} else {
		for _, c := range comments {
			buf.WriteString(fmt.Sprintf("--- %s (%s) ---\n%s\n\n", c.Author, c.CreatedAt, c.Body))
		}
	}

	w.Addr(",")
	w.Write("data", []byte(buf.String()))
	w.Ctl("clean")
	w.Addr("0")
	w.Ctl("dot=addr")
	w.Ctl("show")
}

func getBead(beadID string, mount string) (*Bead, error) {
	if !isConnected() {
		return nil, fmt.Errorf("not connected to anvilsrv")
	}

	data, err := readFile(filepath.Join("beads", mount, beadID, "json"))
	if err != nil {
		return nil, err
	}

	var bead Bead
	if err := json.Unmarshal(data, &bead); err != nil {
		return nil, err
	}

	return &bead, nil
}

func addBlocksDep(blockerID, blockedID string, mount string) error {
	if !isConnected() {
		return fmt.Errorf("not connected to anvilsrv")
	}
	// dep <child> <parent> means parent blocks child
	// So "blockerID blocks blockedID" means blockedID depends on blockerID
	cmd := fmt.Sprintf("dep %s %s", blockedID, blockerID)
	return writeFile(filepath.Join("beads", mount, "ctl"), []byte(cmd))
}

func removeBlocksDep(beadID, blockerID string, mount string) error {
	if !isConnected() {
		return fmt.Errorf("not connected to anvilsrv")
	}
	cmd := fmt.Sprintf("undep %s %s", beadID, blockerID)
	return writeFile(filepath.Join("beads", mount, "ctl"), []byte(cmd))
}

func deleteBead(beadID string, mount string) error {
	if !isConnected() {
		return fmt.Errorf("not connected to anvilsrv")
	}
	cmd := fmt.Sprintf("delete %s", beadID)
	return writeFile(filepath.Join("beads", mount, "ctl"), []byte(cmd))
}

func claimBeadAsUser(beadID string, mount string) error {
	if !isConnected() {
		return fmt.Errorf("not connected to anvilsrv")
	}
	return writeFile(filepath.Join("beads", mount, "ctl"), []byte(fmt.Sprintf("claim %s user", beadID)))
}

func unclaimBead(beadID string, mount string) error {
	if !isConnected() {
		return fmt.Errorf("not connected to anvilsrv")
	}
	return writeFile(filepath.Join("beads", mount, "ctl"), []byte(fmt.Sprintf("unclaim %s", beadID)))
}

func completeBead(beadID string, mount string) error {
	if !isConnected() {
		return fmt.Errorf("not connected to anvilsrv")
	}
	return writeFile(filepath.Join("beads", mount, "ctl"), []byte(fmt.Sprintf("complete %s", beadID)))
}

func failBead(beadID, reason string, mount string) error {
	if !isConnected() {
		return fmt.Errorf("not connected to anvilsrv")
	}
	return writeFile(filepath.Join("beads", mount, "ctl"), []byte(fmt.Sprintf("fail %s %s", beadID, reason)))
}

func updateBead(beadID, content string, origBlockers []string, mount string) error {
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
	requiresApproval := -1 // -1=unset, 0=false, 1=true
	requiresReview := -1
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
		} else if strings.HasPrefix(line, "requires_approval:") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "requires_approval:"))
			if val == "yes" || val == "true" || val == "1" {
				requiresApproval = 1
			} else {
				requiresApproval = 0
			}
		} else if strings.HasPrefix(line, "requires_review:") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "requires_review:"))
			if val == "yes" || val == "true" || val == "1" {
				requiresReview = 1
			} else {
				requiresReview = 0
			}
		}
	}

	// Update title and description
	if title != "" {
		cmd := fmt.Sprintf("update %s title '%s'", beadID, title)
		writeFile(filepath.Join("beads", mount, "ctl"), []byte(cmd))
	}
	if description != "" {
		cmd := fmt.Sprintf("update %s description '%s'", beadID, description)
		writeFile(filepath.Join("beads", mount, "ctl"), []byte(cmd))
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
			addBlocksDep(b, beadID, mount)
		}
	}
	for _, b := range origBlockers {
		if !newSet[b] {
			removeBlocksDep(beadID, b, mount)
		}
	}

	// Toggle requires_approval label
	if requiresApproval == 1 {
		writeFile(filepath.Join("beads", mount, "ctl"), []byte(fmt.Sprintf("label %s requires_approval", beadID)))
	} else if requiresApproval == 0 {
		writeFile(filepath.Join("beads", mount, "ctl"), []byte(fmt.Sprintf("unlabel %s requires_approval", beadID)))
	}

	// Toggle requires_review label
	if requiresReview == 1 {
		writeFile(filepath.Join("beads", "", "ctl"), []byte(fmt.Sprintf("label %s requires_review", beadID)))
	} else if requiresReview == 0 {
		writeFile(filepath.Join("beads", "", "ctl"), []byte(fmt.Sprintf("unlabel %s requires_review", beadID)))
	}

	return nil
}

func initBeads(prefix string, mount string) error {
	if !isConnected() {
		return fmt.Errorf("not connected to anvilsrv")
	}
	return writeFile(filepath.Join("beads", mount, "ctl"), []byte(fmt.Sprintf("init %s", prefix)))
}

// mountProject mounts cwd via mount_beads.sh mcptool and returns the generated mount name.
func mountProject(cwd string) (string, error) {
	script, err := readFile("tools/mount_beads.sh")
	if err != nil {
		return "", fmt.Errorf("read mount_beads.sh: %w", err)
	}
	cmd := exec.Command("bash", "-s", "--", cwd)
	cmd.Stdin = bytes.NewReader(script)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("mount_beads.sh: %w", err)
	}
	name := strings.TrimSpace(string(out))
	if name == "" {
		return "", fmt.Errorf("mount_beads.sh returned empty mount name")
	}
	return name, nil
}

func umountProject(name string) error {
	if !isConnected() {
		return fmt.Errorf("not connected to anvilsrv")
	}
	return writeFile("beads/ctl", []byte(fmt.Sprintf("umount %s", name)))
}
