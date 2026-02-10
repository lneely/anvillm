// Q - Acme interface for chat backends via 9P
package main

import (
	"anvillm/internal/backend"
	"anvillm/internal/backend/tmux"
	"anvillm/internal/backends"
	"anvillm/internal/debug"
	"anvillm/internal/p9"
	"anvillm/internal/session"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"9fans.net/go/acme"
)

const windowName = "/AnviLLM/"

// Terminal to use for tmux attach (configurable)
// Common options: "foot", "kitty", "xterm", "st", "alacritty"
const terminalCommand = "foot"

// Type alias to work around Go parser limitation
type Session = backend.Session

func main() {
	flag.Parse()

	// Create all backends
	backendMap := map[string]backend.Backend{
		"kiro-cli": backends.NewKiroCLI(),
		"claude":   backends.NewClaude(),
	}

	mgr := session.NewManager(backendMap)

	// Cleanup tmux sessions on exit
	defer func() {
		for _, b := range backendMap {
			if tmuxBackend, ok := b.(*tmux.Backend); ok {
				tmuxBackend.Cleanup()
			}
		}
	}()

	srv, err := p9.NewServer(mgr)
	if err != nil {
		log.Fatal(err)
	}
	defer srv.Close()

	// Rename chat window when alias changes
	var aliasChangeFunc func(Session)
	aliasChangeFunc = func(sess Session) {
		meta := sess.Metadata()
		if meta.WinID > 0 {
			w, err := acme.Open(meta.WinID, nil)
			if err != nil {
				return
			}
			defer w.CloseFiles()
			name := meta.Alias
			if name == "" {
				name = sess.ID()
			}
			w.Name(filepath.Join(meta.Cwd, fmt.Sprintf("+Chat.%s", name)))
		}
	}
	srv.OnAliasChange = aliasChangeFunc

	w, err := acme.New()
	if err != nil {
		log.Fatal(err)
	}
	defer w.CloseFiles()

	w.Name(windowName)
	w.Write("tag", []byte("Kiro Claude Open Kill Get Login "))
	refreshList(w, mgr)
	w.Ctl("clean")

	// Periodic refresh
	go func() {
		for range time.Tick(5 * time.Second) {
			refreshList(w, mgr)
		}
	}()

	// Event loop
	for e := range w.EventChan() {
		switch e.C2 {
		case 'x', 'X':
			cmd := string(e.Text)
			arg := strings.TrimSpace(string(e.Arg))

			// Handle "Kiro /path", "Claude /path", "Kill <pid>", "Open <id>" typed directly in tag
			if strings.HasPrefix(cmd, "Kiro ") {
				arg = strings.TrimPrefix(cmd, "Kiro ")
				cmd = "Kiro"
			} else if strings.HasPrefix(cmd, "Claude ") {
				arg = strings.TrimPrefix(cmd, "Claude ")
				cmd = "Claude"
			} else if strings.HasPrefix(cmd, "Kill ") {
				arg = strings.TrimPrefix(cmd, "Kill ")
				cmd = "Kill"
			} else if strings.HasPrefix(cmd, "Open ") {
				arg = strings.TrimPrefix(cmd, "Open ")
				cmd = "Open"
			}

			switch cmd {
			case "Kiro":
				// Require path argument
				if arg == "" {
					fmt.Fprintf(os.Stderr, "Error: Kiro requires a path argument\n")
					continue
				}
				sess, err := mgr.New("kiro-cli", arg)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					continue
				}
				_, err = openChatWindow(sess)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error opening chat: %v\n", err)
					continue
				}
				// WinID is set inside openChatWindow
				refreshList(w, mgr)
			case "Claude":
				// Require path argument
				if arg == "" {
					fmt.Fprintf(os.Stderr, "Error: Claude requires a path argument\n")
					continue
				}
				sess, err := mgr.New("claude", arg)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					continue
				}
				_, err = openChatWindow(sess)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error opening chat: %v\n", err)
					continue
				}
				// WinID is set inside openChatWindow
				refreshList(w, mgr)
			case "Open":
				if arg == "" {
					fmt.Fprintf(os.Stderr, "Usage: Open <session-id>\n")
					continue
				}
				sess := mgr.Get(arg)
				if sess == nil {
					fmt.Fprintf(os.Stderr, "Session not found: %s\n", arg)
					continue
				}
				_, err := openChatWindow(sess)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error opening chat: %v\n", err)
					continue
				}
				// WinID is set inside openChatWindow
			case "Kill":
				if arg == "" {
					fmt.Fprintf(os.Stderr, "Usage: Kill <pid>\n")
					continue
				}
				pid, err := strconv.Atoi(arg)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Invalid pid: %s\n", arg)
					continue
				}
				// Find session by pid and kill it
				for _, id := range mgr.List() {
					sess := mgr.Get(id)
					if sess != nil {
						meta := sess.Metadata()
						if meta.Pid == pid {
							// First kill the process
							if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
								debug.Log("[session %s] Failed to kill PID %d: %v", id, pid, err)
							} else {
								debug.Log("[session %s] Killed PID %d", id, pid)
							}
							// Then close the tmux window
							sess.Close()
							mgr.Remove(id)
							break
						}
					}
				}
				refreshList(w, mgr)
			case "Get":
				refreshList(w, mgr)
			case "Debug":
				debug.Enabled = !debug.Enabled
				state := "disabled"
				if debug.Enabled {
					state = "enabled"
				}
				fmt.Printf("Debug mode %s\n", state)
			case "Login":
				cmd := exec.Command("firefox")
				cmd.Start()
				cmd = exec.Command("foot", "-e", "q", "login")
				cmd.Start()
			default:
				w.WriteEvent(e)
			}
		case 'l', 'L':
			text := strings.TrimSpace(string(e.Text))
			if sess := mgr.Get(text); sess != nil {
				// Try to focus existing window, or open new one
				meta := sess.Metadata()
				if meta.WinID > 0 {
					// Check if window still exists by trying to open it
					if aw, err := acme.Open(meta.WinID, nil); err == nil {
						aw.Ctl("show")
						aw.CloseFiles()
					} else {
						// Window gone, open new one
						openChatWindow(sess)
					}
				} else {
					openChatWindow(sess)
				}
			} else {
				w.WriteEvent(e)
			}
		default:
			w.WriteEvent(e)
		}
	}
}

func refreshList(w *acme.Win, mgr *session.Manager) {
	var buf strings.Builder
	// Header: ID=5, Backend=10, State=9, PID=8, Alias=16, Cwd=remaining
	buf.WriteString(fmt.Sprintf("%-5s %-10s %-9s %-8s %-16s %s\n", "ID", "Backend", "State", "PID", "Alias", "Cwd"))
	buf.WriteString(fmt.Sprintf("%-5s %-10s %-9s %-8s %-16s %s\n", "-----", "----------", "---------", "--------", "----------------", strings.Repeat("-", 40)))
	for _, id := range mgr.List() {
		sess := mgr.Get(id)
		if sess == nil {
			continue
		}
		meta := sess.Metadata()
		// Remove dead sessions (pid=0 or process not running)
		if meta.Pid == 0 {
			mgr.Remove(id)
			continue
		}
		if err := syscall.Kill(meta.Pid, 0); err != nil {
			mgr.Remove(id)
			continue
		}
		alias := meta.Alias
		if alias == "" {
			alias = "-"
		}
		cwd := meta.Cwd
		if len(cwd) > 80 {
			cwd = "..." + cwd[len(cwd)-77:]
		}
		buf.WriteString(fmt.Sprintf("%-5s %-10s %-9s %-8d %-16s %s\n", sess.ID(), meta.Backend, sess.State(), meta.Pid, alias, cwd))
	}
	w.Addr(",")
	w.Write("data", []byte(buf.String()))
	w.Ctl("clean")
}

func openChatWindow(sess backend.Session) (*acme.Win, error) {
	meta := sess.Metadata()
	displayName := meta.Alias
	if displayName == "" {
		displayName = sess.ID()
	}
	name := filepath.Join(meta.Cwd, fmt.Sprintf("+Chat.%s", displayName))

	w, err := acme.New()
	if err != nil {
		return nil, err
	}
	w.Name(name)
	w.Write("tag", []byte("Send Stop Attach Alias Save Sessions "))
	w.Fprintf("body", "# Session %s\n# cwd: %s\n\nUSER:\n", sess.ID(), meta.Cwd)
	w.Ctl("clean")

	// Set window ID on session (if it's a tmux session)
	if tmuxSess, ok := sess.(*tmux.Session); ok {
		tmuxSess.SetWinID(w.ID())
	}

	go handleChatWindow(w, sess)

	return w, nil
}

func handleChatWindow(w *acme.Win, sess backend.Session) {
	defer w.CloseFiles()

	for e := range w.EventChan() {
		switch e.C2 {
		case 'x', 'X':
			cmd := string(e.Text)
			arg := strings.TrimSpace(string(e.Arg))

			// Handle "Alias <name>" typed directly
			if strings.HasPrefix(cmd, "Alias ") {
				arg = strings.TrimPrefix(cmd, "Alias ")
				cmd = "Alias"
			}

			switch cmd {
			case "Send":
				go sendPrompt(w, sess)
			case "Stop":
				ctx := context.Background()
				if err := sess.Stop(ctx); err != nil {
					fmt.Fprintf(os.Stderr, "Stop error: %v\n", err)
				} else {
					debug.Log("[session %s] Sent CTRL+C", sess.ID())
				}
			case "Attach":
				// Attach to tmux window (only works for tmux-based backends)
				meta := sess.Metadata()
				tmuxSession, hasSession := meta.Extra["tmux_session"]
				tmuxWindow, hasWindow := meta.Extra["tmux_window"]
				if hasSession && hasWindow {
					go func() {
						// Attach directly to the specific window
						target := fmt.Sprintf("%s:%s", tmuxSession, tmuxWindow)
						cmd := exec.Command(terminalCommand, "-e", "tmux", "attach", "-t", target)
						if err := cmd.Start(); err != nil {
							fmt.Fprintf(os.Stderr, "Failed to launch terminal: %v\n", err)
						} else {
							debug.Log("[session %s] Launched terminal attached to window: %s", sess.ID(), target)
						}
					}()
				} else {
					fmt.Fprintf(os.Stderr, "Attach not supported: not a tmux-based backend\n")
				}
			case "Alias":
				if arg == "" {
					fmt.Fprintf(os.Stderr, "Usage: Alias <name>\n")
					continue
				}
				// Validate alias
				matched, _ := regexp.MatchString(`^[A-Za-z0-9_-]+$`, arg)
				if !matched {
					fmt.Fprintf(os.Stderr, "Invalid alias: must match [A-Za-z0-9_-]+\n")
					continue
				}
				sess.SetAlias(arg)
				// Rename window
				meta := sess.Metadata()
				newName := filepath.Join(meta.Cwd, fmt.Sprintf("+Chat.%s", arg))
				w.Name(newName)
				fmt.Printf("Alias set to %s\n", arg)
			case "Save":
				// Grab first part of window content for filename hints
				var bodyText string
				if body, err := w.ReadAll("body"); err == nil && len(body) > 0 {
					if len(body) > 500 {
						body = body[:500]
					}
					bodyText = string(body)
				}
				go func() {
					var savePath string
					var err error
					if arg != "" {
						// Save to explicit path
						savePath = arg
						err = backends.SendCtl(sess, "/chat save "+savePath)
					} else {
						// Save with smart filename
						savePath, err = backends.SaveWithSmartFilename(sess, sess.ID(), bodyText)
					}
					if err != nil {
						fmt.Fprintf(os.Stderr, "Error saving: %v\n", err)
						return
					}
					fmt.Printf("Saved to %s\n", savePath)
				}()
			case "Sessions":
				openSessionsWindow(w, sess)
			default:
				w.WriteEvent(e)
			}
		default:
			w.WriteEvent(e)
		}
	}
}

func sendPrompt(w *acme.Win, sess backend.Session) {
	body, err := w.ReadAll("body")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading body: %v\n", err)
		return
	}

	content := string(body)
	idx := strings.LastIndex(content, "USER:")
	if idx < 0 {
		fmt.Fprintf(os.Stderr, "No USER: section found\n")
		return
	}

	prompt := strings.TrimSpace(content[idx+5:])
	if prompt == "" {
		return
	}

	w.Fprintf("body", "\n\nASSISTANT:\n")

	ctx := context.Background()
	response, err := sess.Send(ctx, prompt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	} else {
		w.Fprintf("body", "%s\n", response)
	}

	w.Fprintf("body", "\n---\n\nUSER:\n")
	w.Ctl("clean")
}

func openSessionsWindow(chatWin *acme.Win, sess backend.Session) {
	dir := backends.SessionsDir()
	meta := sess.Metadata()
	name := filepath.Join(meta.Cwd, fmt.Sprintf("+Sessions.%s", sess.ID()))

	w, err := acme.New()
	if err != nil {
		return
	}
	w.Name(name)
	w.Write("tag", []byte("Get "))

	entries, _ := os.ReadDir(dir)
	w.Fprintf("body", "Saved sessions in %s:\n\n", dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			w.Fprintf("body", "%s\n", filepath.Join(dir, e.Name()))
		}
	}
	w.Ctl("clean")

	go func() {
		defer w.CloseFiles()
		for e := range w.EventChan() {
			if e.C2 == 'l' || e.C2 == 'L' {
				path := strings.TrimSpace(string(e.Text))
				if strings.HasSuffix(path, ".json") {
					// Run /chat load and display output in chat window
					go func(p string) {
						chatWin.Fprintf("body", "\n\nASSISTANT:\n")
						ctx := context.Background()
						response, err := sess.Send(ctx, "/chat load "+p)
						if err != nil {
							fmt.Fprintf(os.Stderr, "Error loading: %v\n", err)
						} else {
							chatWin.Fprintf("body", "%s\n", response)
						}
						chatWin.Fprintf("body", "\n---\n\nUSER:\n")
						chatWin.Ctl("clean")
					}(path)
				}
			} else if e.C2 == 'x' || e.C2 == 'X' {
				if string(e.Text) == "Get" {
					w.Addr(",")
					entries, _ := os.ReadDir(dir)
					var buf strings.Builder
					buf.WriteString(fmt.Sprintf("Saved sessions in %s:\n\n", dir))
					for _, ent := range entries {
						if strings.HasSuffix(ent.Name(), ".json") {
							buf.WriteString(filepath.Join(dir, ent.Name()) + "\n")
						}
					}
					w.Write("data", []byte(buf.String()))
					w.Ctl("clean")
				} else {
					w.WriteEvent(e)
				}
			} else {
				w.WriteEvent(e)
			}
		}
	}()
}

func focusWindow(id int) {
	w, err := acme.Open(id, nil)
	if err != nil {
		return
	}
	defer w.CloseFiles()
	w.Ctl("show")
}
