// Q - Acme interface for chat backends via 9P
package main

import (
	"anvillm/internal/backend"
	"anvillm/internal/backend/tmux"
	"anvillm/internal/backends"
	"anvillm/internal/debug"
	"anvillm/internal/p9"
	"anvillm/internal/session"
	"anvillm/internal/ui"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

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

	// Start notification daemon in background
	notifyCmd := exec.Command("anvillm-notify")
	notifyCmd.Start()
	defer func() {
		if notifyCmd.Process != nil {
			notifyCmd.Process.Kill()
		}
	}()

	// Rename prompt window when alias changes
	srv.OnAliasChange = func(sess Session) {
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
			w.Name(filepath.Join(meta.Cwd, fmt.Sprintf("+Prompt.%s", name)))
		}
	}

	w, err := acme.New()
	if err != nil {
		log.Fatal(err)
	}
	defer w.CloseFiles()

	w.Name(windowName)
	w.Write("tag", []byte("Kiro Claude Open Kill Attach Alias Context Get Sandbox "))
	refreshList(w, mgr)
	w.Ctl("clean")

	// Event loop
	for e := range w.EventChan() {
		switch e.C2 {
		case 'x', 'X':
			cmd := string(e.Text)
			arg := strings.TrimSpace(string(e.Arg))

			// Fire-and-forget: B2 on session ID sends selected text as prompt
			// In a 2-1 chord, the selection comes through as e.Arg
			if matched, _ := regexp.MatchString(`^[a-f0-9]{8}$`, cmd); matched {
				if sess := mgr.Get(cmd); sess != nil {
					if len(e.Arg) > 0 {
						go sess.Send(context.Background(), string(e.Arg))
					}
				}
				continue
			}

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
			} else if strings.HasPrefix(cmd, "Alias ") {
				arg = strings.TrimPrefix(cmd, "Alias ")
				cmd = "Alias"
			} else if strings.HasPrefix(cmd, "Context ") {
				arg = strings.TrimPrefix(cmd, "Context ")
				cmd = "Context"
			}

			switch cmd {
			case "Kiro":
				// Require path argument
				if arg == "" {
					fmt.Fprintf(os.Stderr, "Error: Kiro requires a path argument\n")
					continue
				}
				_, err := mgr.New("kiro-cli", arg)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					continue
				}
				refreshList(w, mgr)
			case "Claude":
				// Require path argument
				if arg == "" {
					fmt.Fprintf(os.Stderr, "Error: Claude requires a path argument\n")
					continue
				}
				_, err := mgr.New("claude", arg)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					continue
				}
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
				_, err := openPromptWindow(sess)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error opening prompt: %v\n", err)
					continue
				}
			case "Kill":
				if arg == "" {
					fmt.Fprintf(os.Stderr, "Usage: Kill <session-id>\n")
					continue
				}
				sess := mgr.Get(arg)
				if sess == nil {
					fmt.Fprintf(os.Stderr, "Session not found: %s\n", arg)
					continue
				}
				sess.Close()
				mgr.Remove(arg)
				refreshList(w, mgr)
			case "Attach":
				if arg == "" {
					fmt.Fprintf(os.Stderr, "Usage: Attach <session-id>\n")
					continue
				}
				sess := mgr.Get(arg)
				if sess == nil {
					fmt.Fprintf(os.Stderr, "Session not found: %s\n", arg)
					continue
				}
				meta := sess.Metadata()
				tmuxSession, hasSession := meta.Extra["tmux_session"]
				tmuxWindow, hasWindow := meta.Extra["tmux_window"]
				if hasSession && hasWindow {
					go func() {
						target := fmt.Sprintf("%s:%s", tmuxSession, tmuxWindow)
						cmd := exec.Command(terminalCommand, "-e", "tmux", "attach", "-t", target)
						if err := cmd.Start(); err != nil {
							fmt.Fprintf(os.Stderr, "Failed to launch terminal: %v\n", err)
						}
					}()
				} else {
					fmt.Fprintf(os.Stderr, "Attach not supported: not a tmux-based backend\n")
				}
			case "Get":
				refreshList(w, mgr)
			case "Alias":
				parts := strings.Fields(arg)
				if len(parts) < 2 {
					fmt.Fprintf(os.Stderr, "Usage: Alias <session-id> <name>\n")
					continue
				}
				sess := mgr.Get(parts[0])
				if sess == nil {
					fmt.Fprintf(os.Stderr, "Session not found: %s\n", parts[0])
					continue
				}
				alias := parts[1]
				matched, _ := regexp.MatchString(`^[A-Za-z0-9_-]+$`, alias)
				if !matched {
					fmt.Fprintf(os.Stderr, "Invalid alias: must match [A-Za-z0-9_-]+\n")
					continue
				}
				sess.SetAlias(alias)
				if srv.OnAliasChange != nil {
					srv.OnAliasChange(sess)
				}
				refreshList(w, mgr)
			case "Context":
				if arg == "" {
					fmt.Fprintf(os.Stderr, "Usage: Context <session-id>\n")
					continue
				}
				sess := mgr.Get(arg)
				if sess == nil {
					fmt.Fprintf(os.Stderr, "Session not found: %s\n", arg)
					continue
				}
				if err := openContextWindow(sess); err != nil {
					fmt.Fprintf(os.Stderr, "Error opening context window: %v\n", err)
				}
			case "Debug":
				debug.Enabled = !debug.Enabled
				state := "disabled"
				if debug.Enabled {
					state = "enabled"
				}
				fmt.Printf("Debug mode %s\n", state)
			case "Sandbox":
				if err := ui.OpenSandboxWindow(); err != nil {
					fmt.Fprintf(os.Stderr, "Error opening sandbox window: %v\n", err)
				}

			default:
				w.WriteEvent(e)
			}
		case 'l', 'L':
			text := strings.TrimSpace(string(e.Text))
			if sess := mgr.Get(text); sess != nil {
				meta := sess.Metadata()
				if meta.WinID > 0 {
					if aw, err := acme.Open(meta.WinID, nil); err == nil {
						aw.Ctl("show")
						aw.CloseFiles()
					} else {
						openPromptWindow(sess)
					}
				} else {
					openPromptWindow(sess)
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

func openPromptWindow(sess backend.Session) (*acme.Win, error) {
	meta := sess.Metadata()
	displayName := meta.Alias
	if displayName == "" {
		displayName = sess.ID()
	}
	name := filepath.Join(meta.Cwd, fmt.Sprintf("+Prompt.%s", displayName))

	w, err := acme.New()
	if err != nil {
		return nil, err
	}
	w.Name(name)
	w.Write("tag", []byte("Send "))
	w.Ctl("clean")

	if tmuxSess, ok := sess.(*tmux.Session); ok {
		tmuxSess.SetWinID(w.ID())
	}

	go handlePromptWindow(w, sess)
	return w, nil
}

func handlePromptWindow(w *acme.Win, sess backend.Session) {
	defer w.CloseFiles()

	for e := range w.EventChan() {
		if (e.C2 == 'x' || e.C2 == 'X') && string(e.Text) == "Send" {
			body, err := w.ReadAll("body")
			if err != nil {
				continue
			}
			prompt := strings.TrimSpace(string(body))
			if prompt != "" {
				go sess.Send(context.Background(), prompt)
				w.Ctl("delete")
				return
			}
		} else {
			w.WriteEvent(e)
		}
	}
}

func openContextWindow(sess backend.Session) error {
	w, err := acme.New()
	if err != nil {
		return err
	}
	w.Name(fmt.Sprintf("/AnviLLM/%s/context", sess.ID()))
	w.Write("tag", []byte("Put "))

	// Load existing context
	if tmuxSess, ok := sess.(*tmux.Session); ok {
		if ctx := tmuxSess.GetContext(); ctx != "" {
			w.Write("body", []byte(ctx))
		}
	}
	w.Ctl("clean")

	go handleContextWindow(w, sess)
	return nil
}

func handleContextWindow(w *acme.Win, sess backend.Session) {
	defer w.CloseFiles()

	for e := range w.EventChan() {
		if e.C2 == 'x' || e.C2 == 'X' {
			if string(e.Text) == "Put" {
				// Read body and write to session context
				body, err := w.ReadAll("body")
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error reading body: %v\n", err)
					continue
				}
				if tmuxSess, ok := sess.(*tmux.Session); ok {
					tmuxSess.SetContext(strings.TrimSpace(string(body)))
					w.Ctl("clean")
					fmt.Printf("Context updated for session %s\n", sess.ID())
				}
				continue
			}
		}
		w.WriteEvent(e)
	}
}

func focusWindow(id int) {
	w, err := acme.Open(id, nil)
	if err != nil {
		return
	}
	defer w.CloseFiles()
	w.Ctl("show")
}
