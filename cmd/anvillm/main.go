// anvillm - Terminal UI for AnviLLM
package main

import (
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
		SetSelectable(true, false)
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
		switch event.Key() {
		case tcell.KeyRune:
			switch event.Rune() {
			case 'q':
				app.Stop()
				return nil
			case 'r':
				refreshSessions()
				return nil
			case 'k':
				showCreateSession("kiro-cli")
				return nil
			case 'c':
				showCreateSession("claude")
				return nil
			case 'p':
				showPromptDialog()
				return nil
			case 's':
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
			case 'd':
				showDaemonStatus()
				return nil
			case '?':
				showHelp()
				return nil
			}
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

	// Header
	sessionList.SetCell(0, 0, tview.NewTableCell("ID").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	sessionList.SetCell(0, 1, tview.NewTableCell("Alias").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	sessionList.SetCell(0, 2, tview.NewTableCell("Backend").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	sessionList.SetCell(0, 3, tview.NewTableCell("State").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	sessionList.SetCell(0, 4, tview.NewTableCell("PID").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	sessionList.SetCell(0, 5, tview.NewTableCell("Cwd").SetTextColor(tcell.ColorYellow).SetSelectable(false))

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

		sessionList.SetCell(row, 0, tview.NewTableCell(sess.ID[:8]))
		sessionList.SetCell(row, 1, tview.NewTableCell(alias))
		sessionList.SetCell(row, 2, tview.NewTableCell(sess.Backend))
		sessionList.SetCell(row, 3, tview.NewTableCell(sess.State).SetTextColor(stateColor))
		sessionList.SetCell(row, 4, tview.NewTableCell(fmt.Sprintf("%d", sess.Pid)))
		sessionList.SetCell(row, 5, tview.NewTableCell(sess.Cwd))
	}

	updateStatus(fmt.Sprintf("Sessions: %d | [yellow]r[white]:refresh [yellow]k[white]:kiro [yellow]c[white]:claude [yellow]p[white]:prompt [yellow]s[white]:stop [yellow]K[white]:kill [yellow]a[white]:alias [yellow]?[white]:help [yellow]q[white]:quit", len(sessions)))
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

func writeFile(path string, data []byte) error {
	fid, err := fs.Open(path, plan9.OWRITE)
	if err != nil {
		return err
	}
	defer fid.Close()

	_, err = fid.Write(data)
	return err
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

func showCreateSession(backend string) {
	input := tview.NewInputField().
		SetLabel(fmt.Sprintf("Create %s session (directory): ", backend)).
		SetFieldWidth(50)

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

	pages.AddPage("input", createModal(input, 60, 3), true, true)
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
			path := filepath.Join(sess.ID, "in")
			if err := writeFile(path, []byte(prompt)); err != nil {
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

	form.SetBorder(true).SetTitle(fmt.Sprintf(" Send Prompt to %s ", sess.ID[:8]))
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
		SetFieldWidth(30)

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

	pages.AddPage("alias", createModal(input, 50, 3), true, true)
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

func showDaemonStatus() {
	// TODO: Implement daemon status view
	updateStatus("[yellow]Daemon status view not yet implemented")
}

func showHelp() {
	helpText := `
[yellow]AnviLLM TUI - Keyboard Shortcuts[-]

[yellow]Session Management:[-]
  k       Create Kiro session
  c       Create Claude session
  s       Stop selected session
  R       Restart selected session
  K       Kill selected session
  a       Set session alias

[yellow]Interaction:[-]
  p       Send prompt to session
  r       Refresh session list

[yellow]Navigation:[-]
  ↑/↓     Select session
  Enter   (reserved)

[yellow]Other:[-]
  d       Daemon status
  ?       Show this help
  q       Quit

[yellow]9P Filesystem:[-]
All operations read/write the 9P filesystem at $NAMESPACE/agent
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

	pages.AddPage("help", createModal(textView, 70, 25), true, true)
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
