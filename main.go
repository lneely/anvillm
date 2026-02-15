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
	"9fans.net/go/plan9/client"
)

const windowName = "/AnviLLM/"

// Terminal to use for tmux attach (configurable)
// Common options: "foot", "kitty", "xterm", "st", "alacritty"
const terminalCommand = "foot"

// Type alias to work around Go parser limitation
type Session = backend.Session

// getNamespaceSuffix extracts the display number from namespace (e.g., "0" from "/tmp/ns.user.:0")
func getNamespaceSuffix() string {
	ns := client.Namespace()
	if ns == "" {
		return ""
	}
	// Extract :N from /tmp/ns.user.:N
	parts := strings.Split(ns, ":")
	if len(parts) >= 2 {
		suffix := parts[len(parts)-1]
		// Remove trailing slash if present
		return strings.TrimSuffix(suffix, "/")
	}
	return ""
}

func main() {
	flag.Parse()

	// Create all backends
	nsSuffix := getNamespaceSuffix()
	backendMap := map[string]backend.Backend{
		"kiro-cli": backends.NewKiroCLI(nsSuffix),
		"claude":   backends.NewClaude(nsSuffix),
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

	// Rename prompt window when alias changes
	srv.OnAliasChange = func(sess backend.Session) {
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
	w.Write("tag", []byte("Get Attach Stop Restart Kill Refresh Alias Context Sandbox "))
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
			} else if strings.HasPrefix(cmd, "Stop ") {
				arg = strings.TrimPrefix(cmd, "Stop ")
				cmd = "Stop"
			} else if strings.HasPrefix(cmd, "Restart ") {
				arg = strings.TrimPrefix(cmd, "Restart ")
				cmd = "Restart"
			} else if strings.HasPrefix(cmd, "Kill ") {
				arg = strings.TrimPrefix(cmd, "Kill ")
				cmd = "Kill"
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
				opts := backend.SessionOptions{CWD: arg}
				_, err := mgr.New(opts, "kiro-cli")
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
				opts := backend.SessionOptions{CWD: arg}
				_, err := mgr.New(opts, "claude")
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					continue
				}
				refreshList(w, mgr)
			case "Stop":
				if arg == "" {
					fmt.Fprintf(os.Stderr, "Usage: Stop <session-id>\n")
					continue
				}
				sess := mgr.Get(arg)
				if sess == nil {
					fmt.Fprintf(os.Stderr, "Session not found: %s\n", arg)
					continue
				}
				if err := sess.Stop(context.Background()); err != nil {
					fmt.Fprintf(os.Stderr, "Failed to stop session: %v\n", err)
					continue
				}
				refreshList(w, mgr)
			case "Restart":
				if arg == "" {
					fmt.Fprintf(os.Stderr, "Usage: Restart <session-id>\n")
					continue
				}
				sess := mgr.Get(arg)
				if sess == nil {
					fmt.Fprintf(os.Stderr, "Session not found: %s\n", arg)
					continue
				}
				if err := sess.Restart(context.Background()); err != nil {
					fmt.Fprintf(os.Stderr, "Failed to restart session: %v\n", err)
					continue
				}
				refreshList(w, mgr)
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
			case "Refresh":
				if arg == "" {
					// Refresh all sessions
					for _, id := range mgr.List() {
						if sess := mgr.Get(id); sess != nil {
							sess.Refresh(context.Background())
						}
					}
				} else {
					sess := mgr.Get(arg)
					if sess == nil {
						fmt.Fprintf(os.Stderr, "Session not found: %s\n", arg)
						continue
					}
					sess.Refresh(context.Background())
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
	// Backends line
	buf.WriteString("Backends: [Kiro] [Claude]\n\n")
	// Header: ID=5, Backend=10, State=9, Alias=16, Cwd=remaining
	buf.WriteString(fmt.Sprintf("%-5s %-10s %-9s %-16s %s\n", "ID", "Backend", "State", "Alias", "Cwd"))
	buf.WriteString(fmt.Sprintf("%-5s %-10s %-9s %-16s %s\n", "-----", "----------", "---------", "----------------", strings.Repeat("-", 40)))
	for _, id := range mgr.List() {
		sess := mgr.Get(id)
		if sess == nil {
			continue
		}
		meta := sess.Metadata()
		state := sess.State()

		// Remove only exited sessions (tmux window destroyed by Close())
		if state == "exited" {
			mgr.Remove(id)
			continue
		}

		// Keep stopped sessions visible (pid=0, state="stopped", can be restarted)
		// Keep running sessions (pid!=0)
		// If pid != 0, verify process is still running
		if meta.Pid != 0 {
			if err := syscall.Kill(meta.Pid, 0); err != nil {
				// Process died unexpectedly - trigger Refresh to update state
				fmt.Fprintf(os.Stderr, "Warning: session %s PID %d died unexpectedly, attempting refresh\n", id, meta.Pid)
				if refreshErr := sess.Refresh(context.Background()); refreshErr != nil {
					fmt.Fprintf(os.Stderr, "  Refresh failed: %v\n", refreshErr)
				}
				// Reload metadata after refresh
				meta = sess.Metadata()
				state = sess.State()
			}
		}
		alias := meta.Alias
		if alias == "" {
			alias = "-"
		}
		buf.WriteString(fmt.Sprintf("%-5s %-10s %-9s %-16s %s\n", sess.ID(), meta.Backend, sess.State(), alias, meta.Cwd))
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
