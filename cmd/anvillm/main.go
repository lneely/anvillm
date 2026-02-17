// anvillm - Terminal UI for AnviLLM
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"9fans.net/go/plan9"
	"9fans.net/go/plan9/client"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

var (
	fs  *client.Fsys
	app *tview.Application

	// UI components
	sessionList *tview.Table
	statusBar   *tview.TextView
	promptInput *tview.InputField
	contextView *tview.TextView
	pages       *tview.Pages
)

type SessionInfo struct {
	ID      string
	Alias   string
	Backend string
	State   string
	Pid     int
	Cwd     string
}

func main() {
	// Connect to anvilsrv
	var err error
	fs, err = connectToServer()
	if err != nil {
		log.Fatalf("Failed to connect to anvilsrv: %v\nRun 'anvilsrv start' first", err)
	}
	defer fs.Close()

	// Create application
	app = tview.NewApplication()

	// Create UI components
	setupUI()

	// Start refresh ticker
	go refreshLoop()

	// Run application
	if err := app.SetRoot(pages, true).EnableMouse(true).Run(); err != nil {
		log.Fatal(err)
	}
}

func setupUI() {
	// Session list table
	sessionList = tview.NewTable().
		SetBorders(false).
		SetSelectable(true, false).
		SetFixed(1, 0) // Fix header row
	sessionList.SetBorder(true).SetTitle(" Sessions ").SetTitleAlign(tview.AlignLeft)

	// Status bar
	statusBar = tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)
	statusBar.SetBorder(true).SetTitle(" Status ")
	updateStatus("Connected to anvilsrv")

	// Main layout
	mainLayout := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(sessionList, 0, 1, true).
		AddItem(statusBar, 3, 0, false)

	// Create pages container
	pages = tview.NewPages()
	pages.AddPage("main", mainLayout, true, true)

	// Key bindings for main view
	sessionList.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		row, col := sessionList.GetSelection()

		switch event.Key() {
		case tcell.KeyRune:
			switch event.Rune() {
			case 'q':
				app.Stop()
				return nil
			case 'r':
				refreshSessions()
				return nil
			case 's':
				showBackendSelectionMenu()
				return nil
			case 'p':
				showPromptDialog()
				return nil
			case 't':
				stopSelectedSession()
				return nil
			case 'R':
				restartSelectedSession()
				return nil
			case 'K':
				killSelectedSession()
				return nil
			case 'a':
				showAliasDialog()
				return nil
			case 'l':
				showLogViewer()
				return nil
			case 'd':
				showDaemonStatus()
				return nil
			case '?':
				showHelp()
				return nil
			// Vim keybindings
			case 'j':
				if row < sessionList.GetRowCount()-1 {
					sessionList.Select(row+1, col)
				}
				return nil
			case 'k':
				if row > 1 { // Don't go above first data row
					sessionList.Select(row-1, col)
				}
				return nil
			}
		// Emacs keybindings
		case tcell.KeyCtrlN:
			if row < sessionList.GetRowCount()-1 {
				sessionList.Select(row+1, col)
			}
			return nil
		case tcell.KeyCtrlP:
			if row > 1 {
				sessionList.Select(row-1, col)
			}
			return nil
		}
		return event
	})

	// Initial refresh
	refreshSessions()
}

func connectToServer() (*client.Fsys, error) {
	return client.MountService("agent")
}

func refreshLoop() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		app.QueueUpdateDraw(func() {
			refreshSessions()
		})
	}
}

func refreshSessions() {
	sessions, err := listSessions()
	if err != nil {
		updateStatus(fmt.Sprintf("[red]Error: %v", err))
		return
	}

	sessionList.Clear()

	// Header with blue background and white text
	headerStyle := tcell.StyleDefault.
		Background(tcell.ColorBlue).
		Foreground(tcell.ColorWhite).
		Bold(true)

	sessionList.SetCell(0, 0, tview.NewTableCell(" ID ").
		SetStyle(headerStyle).
		SetSelectable(false).
		SetExpansion(1))
	sessionList.SetCell(0, 1, tview.NewTableCell(" Alias ").
		SetStyle(headerStyle).
		SetSelectable(false).
		SetExpansion(1))
	sessionList.SetCell(0, 2, tview.NewTableCell(" Backend ").
		SetStyle(headerStyle).
		SetSelectable(false).
		SetExpansion(1))
	sessionList.SetCell(0, 3, tview.NewTableCell(" State ").
		SetStyle(headerStyle).
		SetSelectable(false).
		SetExpansion(1))
	sessionList.SetCell(0, 4, tview.NewTableCell(" PID ").
		SetStyle(headerStyle).
		SetSelectable(false).
		SetExpansion(1))
	sessionList.SetCell(0, 5, tview.NewTableCell(" Cwd ").
		SetStyle(headerStyle).
		SetSelectable(false).
		SetExpansion(3))

	// Sessions
	for i, sess := range sessions {
		row := i + 1
		alias := sess.Alias
		if alias == "" {
			alias = "-"
		}

		stateColor := tcell.ColorWhite
		switch sess.State {
		case "running":
			stateColor = tcell.ColorGreen
		case "idle":
			stateColor = tcell.ColorAqua
		case "stopped":
			stateColor = tcell.ColorYellow
		case "error", "exited":
			stateColor = tcell.ColorRed
		}

		sessionList.SetCell(row, 0, tview.NewTableCell(" "+sess.ID[:8]+" ").SetExpansion(1))
		sessionList.SetCell(row, 1, tview.NewTableCell(" "+alias+" ").SetExpansion(1))
		sessionList.SetCell(row, 2, tview.NewTableCell(" "+sess.Backend+" ").SetExpansion(1))
		sessionList.SetCell(row, 3, tview.NewTableCell(" "+sess.State+" ").SetTextColor(stateColor).SetExpansion(1))
		sessionList.SetCell(row, 4, tview.NewTableCell(fmt.Sprintf(" %d ", sess.Pid)).SetExpansion(1))
		sessionList.SetCell(row, 5, tview.NewTableCell(" "+sess.Cwd+" ").SetExpansion(3))
	}

	updateStatus(fmt.Sprintf("Sessions: %d | [yellow]s[white]:start [yellow]p[white]:prompt [yellow]t[white]:stop [yellow]R[white]:restart [yellow]K[white]:kill [yellow]a[white]:alias [yellow]r[white]:refresh [yellow]j/k,C-n/C-p[white]:nav [yellow]?[white]:help [yellow]q[white]:quit", len(sessions)))
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

		// Parse: id alias state pid cwd
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}

		var pid int
		fmt.Sscanf(fields[3], "%d", &pid)

		sess := &SessionInfo{
			ID:      fields[0],
			Alias:   fields[1],
			State:   fields[2],
			Pid:     pid,
			Cwd:     strings.Join(fields[4:], " "),
		}
		if sess.Alias == "-" {
			sess.Alias = ""
		}

		// Read backend
		if backend, err := readFile(filepath.Join(sess.ID, "backend")); err == nil {
			sess.Backend = strings.TrimSpace(string(backend))
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

func readLogFile(path string) ([]byte, error) {
	fid, err := fs.Open(path, plan9.OREAD)
	if err != nil {
		return nil, err
	}
	defer fid.Close()

	// Read in chunks, but stop when we get less than requested
	// This prevents blocking on the streaming log file
	var buf []byte
	tmp := make([]byte, 8192)
	for {
		n, err := fid.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		// Stop if we got less than buffer size (no more data available)
		// or if there was an error
		if n < len(tmp) || err != nil {
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

func sendPrompt(id, prompt string) error {
	// Create message JSON
	msg := map[string]interface{}{
		"to":      id,
		"type":    "PROMPT",
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

func updateStatus(msg string) {
	statusBar.Clear()
	fmt.Fprintf(statusBar, " %s", msg)
}

func getSelectedSession() *SessionInfo {
	row, _ := sessionList.GetSelection()
	if row <= 0 {
		return nil
	}

	sessions, _ := listSessions()
	if row-1 < len(sessions) {
		return sessions[row-1]
	}
	return nil
}

func showBackendSelectionMenu() {
	backends := []string{"claude", "kiro-cli"}

	list := tview.NewList().
		ShowSecondaryText(false)

	for _, backend := range backends {
		b := backend // capture for closure
		list.AddItem(b, "", 0, func() {
			pages.RemovePage("backend-menu")
			showCreateSession(b)
		})
	}

	list.SetBorder(true).
		SetTitle(" Select Backend ").
		SetTitleAlign(tview.AlignLeft).
		SetBorderColor(tcell.ColorBlue)

	list.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape || event.Rune() == 'q' {
			pages.RemovePage("backend-menu")
			return nil
		}
		return event
	})

	pages.AddPage("backend-menu", createModal(list, 30, 5), true, true)
}

func showCreateSession(backend string) {
	input := tview.NewInputField().
		SetLabel("Directory: ").
		SetFieldWidth(0).
		SetFieldTextColor(tcell.ColorBlack).
		SetFieldBackgroundColor(tcell.ColorWhite)

	input.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			dir := input.GetText()
			if dir != "" {
				cmd := fmt.Sprintf("new %s %s", backend, dir)
				if err := writeFile("ctl", []byte(cmd)); err != nil {
					updateStatus(fmt.Sprintf("[red]Error: %v", err))
				} else {
					updateStatus(fmt.Sprintf("[green]Created %s session in %s", backend, dir))
					refreshSessions()
				}
			}
		}
		pages.RemovePage("input")
	})

	// Wrap input in a flex container with border
	container := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(input, 1, 0, true)

	container.SetBorder(true).
		SetTitle(fmt.Sprintf(" Create %s Session ", backend)).
		SetTitleAlign(tview.AlignLeft).
		SetBorderColor(tcell.ColorBlue)

	pages.AddPage("input", createModal(container, 60, 5), true, true)
}

func showPromptDialog() {
	sess := getSelectedSession()
	if sess == nil {
		updateStatus("[yellow]No session selected")
		return
	}

	input := tview.NewTextArea().
		SetPlaceholder("Enter prompt...")

	form := tview.NewForm().
		AddFormItem(input)

	form.AddButton("Send", func() {
		prompt := input.GetText()
		if prompt != "" {
			if err := sendPrompt(sess.ID, prompt); err != nil {
				updateStatus(fmt.Sprintf("[red]Error: %v", err))
			} else {
				updateStatus(fmt.Sprintf("[green]Sent prompt to %s", sess.ID[:8]))
			}
		}
		pages.RemovePage("prompt")
	})

	form.AddButton("Cancel", func() {
		pages.RemovePage("prompt")
	})

	form.SetBorder(true).
		SetTitle(fmt.Sprintf(" Send Prompt to %s ", sess.ID[:8])).
		SetTitleAlign(tview.AlignLeft).
		SetBorderColor(tcell.ColorBlue)

	pages.AddPage("prompt", createModal(form, 80, 20), true, true)
}

func showAliasDialog() {
	sess := getSelectedSession()
	if sess == nil {
		updateStatus("[yellow]No session selected")
		return
	}

	input := tview.NewInputField().
		SetLabel("Alias: ").
		SetText(sess.Alias).
		SetFieldWidth(0).
		SetFieldTextColor(tcell.ColorBlack).
		SetFieldBackgroundColor(tcell.ColorWhite)

	input.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			alias := input.GetText()
			path := filepath.Join(sess.ID, "alias")
			if err := writeFile(path, []byte(alias)); err != nil {
				updateStatus(fmt.Sprintf("[red]Error: %v", err))
			} else {
				updateStatus(fmt.Sprintf("[green]Set alias to '%s'", alias))
				refreshSessions()
			}
		}
		pages.RemovePage("alias")
	})

	// Wrap input in a flex container with border
	container := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(input, 1, 0, true)

	container.SetBorder(true).
		SetTitle(fmt.Sprintf(" Set Alias for %s ", sess.ID[:8])).
		SetTitleAlign(tview.AlignLeft).
		SetBorderColor(tcell.ColorBlue)

	pages.AddPage("alias", createModal(container, 50, 5), true, true)
}

func stopSelectedSession() {
	sess := getSelectedSession()
	if sess == nil {
		updateStatus("[yellow]No session selected")
		return
	}

	path := filepath.Join(sess.ID, "ctl")
	if err := writeFile(path, []byte("stop")); err != nil {
		updateStatus(fmt.Sprintf("[red]Error: %v", err))
	} else {
		updateStatus(fmt.Sprintf("[green]Stopped session %s", sess.ID[:8]))
		refreshSessions()
	}
}

func restartSelectedSession() {
	sess := getSelectedSession()
	if sess == nil {
		updateStatus("[yellow]No session selected")
		return
	}

	path := filepath.Join(sess.ID, "ctl")
	if err := writeFile(path, []byte("restart")); err != nil {
		updateStatus(fmt.Sprintf("[red]Error: %v", err))
	} else {
		updateStatus(fmt.Sprintf("[green]Restarted session %s", sess.ID[:8]))
		refreshSessions()
	}
}

func killSelectedSession() {
	sess := getSelectedSession()
	if sess == nil {
		updateStatus("[yellow]No session selected")
		return
	}

	path := filepath.Join(sess.ID, "ctl")
	if err := writeFile(path, []byte("kill")); err != nil {
		updateStatus(fmt.Sprintf("[red]Error: %v", err))
	} else {
		updateStatus(fmt.Sprintf("[green]Killed session %s", sess.ID[:8]))
		refreshSessions()
	}
}

func showLogViewer() {
	// Read the centralized audit log
	path := "audit"
	logData, err := readLogFile(path)
	if err != nil {
		updateStatus(fmt.Sprintf("[red]Error reading audit log: %v", err))
		return
	}

	logText := string(logData)
	if logText == "" {
		logText = "[gray]No audit log entries yet[-]"
	}

	textView := tview.NewTextView().
		SetDynamicColors(true).
		SetText(logText).
		SetScrollable(true).
		SetWordWrap(true)

	textView.SetBorder(true).
		SetTitle(" Audit Log ").
		SetTitleAlign(tview.AlignLeft)

	// Scroll to bottom
	textView.ScrollToEnd()

	textView.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case 'q':
			pages.RemovePage("log")
			return nil
		case 'r':
			// Refresh log
			logData, err := readLogFile(path)
			if err != nil {
				updateStatus(fmt.Sprintf("[red]Error refreshing log: %v", err))
				return nil
			}
			logText := string(logData)
			if logText == "" {
				logText = "[gray]No audit log entries yet[-]"
			}
			textView.SetText(logText)
			textView.ScrollToEnd()
			return nil
		}
		if event.Key() == tcell.KeyEscape {
			pages.RemovePage("log")
			return nil
		}
		return event
	})

	pages.AddPage("log", createModalDynamic(textView, 8, 25), true, true)
}

func showDaemonStatus() {
	// TODO: Implement daemon status view
	updateStatus("[yellow]Daemon status view not yet implemented")
}

func showHelp() {
	helpText := `
[yellow]AnviLLM TUI - Keyboard Shortcuts[-]

[yellow]Session Management:[-]
  s       Start new session (shows backend menu)
  t       Stop selected session
  R       Restart selected session
  K       Kill selected session
  a       Set session alias

[yellow]Interaction:[-]
  p       Send prompt to session
  l       View session log (press 'r' to refresh, 'q' to close)
  r       Refresh session list

[yellow]Navigation:[-]
  ↑/↓     Select session (arrow keys)
  j/k     Select session (vim-style)
  C-n/C-p Select session (emacs-style)

[yellow]Other:[-]
  d       Daemon status
  ?       Show this help
  q       Quit (or ESC in dialogs)

[yellow]9P Filesystem:[-]
All operations read/write the 9P filesystem at $NAMESPACE/agent

[yellow]Backends:[-]
Press 's' to start a new session and choose from:
  - claude     (Claude Code CLI)
  - kiro-cli   (Kiro CLI)
`

	textView := tview.NewTextView().
		SetDynamicColors(true).
		SetText(helpText).
		SetScrollable(true)

	textView.SetBorder(true).SetTitle(" Help ")
	textView.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Rune() == 'q' || event.Key() == tcell.KeyEscape {
			pages.RemovePage("help")
			return nil
		}
		return event
	})

	pages.AddPage("help", createModal(textView, 70, 30), true, true)
}

func createModal(p tview.Primitive, width, height int) tview.Primitive {
	return tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(p, height, 1, true).
			AddItem(nil, 0, 1, false), width, 1, true).
		AddItem(nil, 0, 1, false)
}

func createModalDynamic(p tview.Primitive, widthProportion, height int) tview.Primitive {
	return tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(p, height, 1, true).
			AddItem(nil, 0, 1, false), 0, widthProportion, true).
		AddItem(nil, 0, 1, false)
}
