// Q - Acme interface for chat backends via 9P
package main

import (
	"acme-q/internal/backend"
	"acme-q/internal/backend/tmux"
	"acme-q/internal/backends"
	"acme-q/internal/p9"
	"acme-q/internal/session"
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

const windowName = "/Q/"

// Terminal to use for tmux attach (configurable)
// Common options: "foot", "kitty", "xterm", "st", "alacritty"
const terminalCommand = "foot"

// Type alias to work around Go parser limitation
type Session = backend.Session

func main() {
	flag.Parse()

	// Determine backend from positional argument, default to kiro-cli
	backendName := "kiro-cli"
	if flag.NArg() > 0 {
		backendName = flag.Arg(0)
	}

	// Create backend
	var backend backend.Backend
	switch backendName {
	case "claude":
		backend = backends.NewClaude()
		log.Printf("Using claude backend")
	case "kiro-cli":
		backend = backends.NewKiroCLI()
		log.Printf("Using kiro-cli backend")
	default:
		log.Fatalf("Unknown backend: %s (available: kiro-cli, claude)", backendName)
	}

	mgr := session.NewManager(backend)

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
	w.Write("tag", []byte("New Open Kill Get Login "))
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

			// Handle "New /path" or "Kill <pid>" typed directly in tag
			if strings.HasPrefix(cmd, "New ") {
				arg = strings.TrimPrefix(cmd, "New ")
				cmd = "New"
			} else if strings.HasPrefix(cmd, "Kill ") {
				arg = strings.TrimPrefix(cmd, "Kill ")
				cmd = "Kill"
			} else if strings.HasPrefix(cmd, "Open ") {
				arg = strings.TrimPrefix(cmd, "Open ")
				cmd = "Open"
			}

			switch cmd {
			case "New":
				// Use current directory if no path specified
				cwd := arg
				if cwd == "" {
					cwd, _ = os.Getwd()
				}
				sess, err := mgr.New(cwd)
				if err != nil {
					w.Fprintf("body", "Error: %v\n", err)
					continue
				}
				_, err = openChatWindow(sess)
				if err != nil {
					w.Fprintf("body", "Error opening chat: %v\n", err)
					continue
				}
				// WinID is set inside openChatWindow
				refreshList(w, mgr)
			case "Open":
				if arg == "" {
					w.Fprintf("body", "Usage: Open <session-id>\n")
					continue
				}
				sess := mgr.Get(arg)
				if sess == nil {
					w.Fprintf("body", "Session not found: %s\n", arg)
					continue
				}
				_, err := openChatWindow(sess)
				if err != nil {
					w.Fprintf("body", "Error opening chat: %v\n", err)
					continue
				}
				// WinID is set inside openChatWindow
			case "Kill":
				if arg == "" {
					w.Fprintf("body", "Usage: Kill <pid>\n")
					continue
				}
				pid, err := strconv.Atoi(arg)
				if err != nil {
					w.Fprintf("body", "Invalid pid: %s\n", arg)
					continue
				}
				// Find session by pid and kill it
				for _, id := range mgr.List() {
					sess := mgr.Get(id)
					if sess != nil {
						meta := sess.Metadata()
						if meta.Pid == pid {
							sess.Close()
							mgr.Remove(id)
							break
						}
					}
				}
				refreshList(w, mgr)
			case "Get":
				refreshList(w, mgr)
			case "Login":
				cmd := exec.Command("firefox")
				cmd.Start()
				cmd = exec.Command("foot", "-e", "q", "login")
				log.Printf("starting terminal, pid will follow")
				if err := cmd.Start(); err != nil {
					log.Printf("terminal start error: %v", err)
				} else {
					log.Printf("terminal started, pid=%d", cmd.Process.Pid)
				}
			default:
				w.WriteEvent(e)
			}
		case 'l', 'L':
			text := strings.TrimSpace(string(e.Text))
			log.Printf("[/Q/] Look: text=%q", text)
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
	// Header: ID=5, State=9, PID=8, Alias=16, Cwd=remaining
	buf.WriteString(fmt.Sprintf("%-5s %-9s %-8s %-16s %s\n", "ID", "State", "PID", "Alias", "Cwd"))
	buf.WriteString(fmt.Sprintf("%-5s %-9s %-8s %-16s %s\n", "-----", "---------", "--------", "----------------", strings.Repeat("-", 40)))
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
		buf.WriteString(fmt.Sprintf("%-5s %-9s %-8d %-16s %s\n", sess.ID(), sess.State(), meta.Pid, alias, cwd))
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
	w.Write("tag", []byte("Send Stop Attach Alias Save Kill Sessions "))
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
					log.Printf("[session %s] Stop error: %v", sess.ID(), err)
				} else {
					log.Printf("[session %s] Sent CTRL+C", sess.ID())
				}
			case "Attach":
				// Attach to tmux session (only works for tmux-based backends)
				meta := sess.Metadata()
				if tmuxSession, ok := meta.Extra["tmux_session"]; ok {
					go func() {
						cmd := exec.Command(terminalCommand, "-e", "tmux", "attach", "-t", tmuxSession)
						if err := cmd.Start(); err != nil {
							log.Printf("[session %s] Failed to launch terminal: %v", sess.ID(), err)
						} else {
							log.Printf("[session %s] Launched terminal attached to tmux session: %s", sess.ID(), tmuxSession)
						}
					}()
				} else {
					log.Printf("[session %s] Attach not supported: not a tmux-based backend", sess.ID())
				}
			case "Alias":
				if arg == "" {
					log.Printf("[session %s] Usage: Alias <name>", sess.ID())
					continue
				}
				// Validate alias
				matched, _ := regexp.MatchString(`^[A-Za-z0-9_-]+$`, arg)
				if !matched {
					log.Printf("[session %s] Invalid alias: must match [A-Za-z0-9_-]+", sess.ID())
					continue
				}
				sess.SetAlias(arg)
				// Rename window
				meta := sess.Metadata()
				newName := filepath.Join(meta.Cwd, fmt.Sprintf("+Chat.%s", arg))
				w.Name(newName)
				log.Printf("[session %s] alias set to %s", sess.ID(), arg)
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
						log.Printf("[session %s] Error saving: %v", sess.ID(), err)
						return
					}
					log.Printf("[session %s] Saved to %s", sess.ID(), savePath)
				}()
			case "Kill":
				sess.Close()
				log.Printf("[session %s] Session killed", sess.ID())
				return
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
		log.Printf("[session %s] Error reading body: %v", sess.ID(), err)
		return
	}

	content := string(body)
	idx := strings.LastIndex(content, "USER:")
	if idx < 0 {
		log.Printf("[session %s] No USER: section found", sess.ID())
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
		log.Printf("[session %s] Error: %v", sess.ID(), err)
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
							chatWin.Fprintf("body", "Error loading: %v\n", err)
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
