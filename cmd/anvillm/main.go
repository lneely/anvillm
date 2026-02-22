// anvillm - Terminal UI for AnviLLM
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
			case ' ':
				showPromptDialog()
				return nil
			case 'T':
				stopSelectedSession()
				return nil
			case 'R':
				restartSelectedSession()
				return nil
			case 'K':
				killSelectedSession()
				return nil
			case 'A':
				showAliasDialog()
				return nil
			case 'c':
				showContextEditor()
				return nil
			case 'i':
				showInbox()
				return nil
			case 'a':
				showArchive()
				return nil
			case 't':
				attachToSession()
				return nil
			case 'b':
				showBeads()
				return nil
			case 'd':
				showDaemonStatus()
				return nil
			case '?':
				showHelp()
				return nil
			// Vim keybindings
			case 'j', 'n':
				if row < sessionList.GetRowCount()-1 {
					sessionList.Select(row+1, col)
				}
				return nil
			case 'k', 'p':
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

	updateStatus(fmt.Sprintf("Sessions: %d | [yellow]s[white]:start [yellow]space[white]:prompt [yellow]c[white]:context [yellow]i[white]:inbox [yellow]a[white]:archive [yellow]b[white]:beads [yellow]t[white]:attach [yellow]T[white]:stop [yellow]R[white]:restart [yellow]K[white]:kill [yellow]A[white]:alias [yellow]r[white]:refresh [yellow]?[white]:help [yellow]q[white]:quit", len(sessions)))
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
		fields := strings.Split(line, "\t")
		if len(fields) < 5 {
			continue
		}

		sess := &SessionInfo{
			ID:      fields[0],
			Backend: fields[1],
			State:   fields[2],
			Alias:   fields[3],
			Cwd:     fields[4],
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
	// Detect and wrap bead IDs
	prompt = wrapBeadIDs(prompt)

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

// inferReplyType returns the appropriate response type for a given request type
func inferReplyType(msgType string) string {
	switch msgType {
	case "APPROVAL_REQUEST":
		return "APPROVAL_RESPONSE"
	case "REVIEW_REQUEST":
		return "REVIEW_RESPONSE"
	case "QUERY_REQUEST":
		return "QUERY_RESPONSE"
	default:
		return "PROMPT_RESPONSE"
	}
}

// completeInboxMessage moves a message from inbox to completed archive
func completeInboxMessage(msgID string) error {
	cmd := fmt.Sprintf("complete %s", msgID)
	return writeFile("user/ctl", []byte(cmd))
}

// sendReply sends a reply message via user/mail
func sendReply(to, replyType, subject, body string) error {
	msg := map[string]interface{}{
		"to":      to,
		"type":    replyType,
		"subject": subject,
		"body":    body,
	}
	msgJSON, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}
	return writeFile("user/mail", msgJSON)
}

// approveMessage sends an approval response and completes the original message
func approveMessage(msg map[string]interface{}) error {
	msgID := ""
	if id, ok := msg["id"].(string); ok {
		msgID = id
	}
	from := ""
	if f, ok := msg["from"].(string); ok {
		from = f
	}
	subject := ""
	if s, ok := msg["subject"].(string); ok {
		subject = "Re: " + s
	}
	msgType := ""
	if t, ok := msg["type"].(string); ok {
		msgType = t
	}

	replyType := inferReplyType(msgType)
	if err := sendReply(from, replyType, subject, "Approved"); err != nil {
		return err
	}

	if msgID != "" {
		return completeInboxMessage(msgID)
	}
	return nil
}

// rejectMessage sends a rejection response and completes the original message
func rejectMessage(msg map[string]interface{}, reason string) error {
	msgID := ""
	if id, ok := msg["id"].(string); ok {
		msgID = id
	}
	from := ""
	if f, ok := msg["from"].(string); ok {
		from = f
	}
	subject := ""
	if s, ok := msg["subject"].(string); ok {
		subject = "Re: " + s
	}
	msgType := ""
	if t, ok := msg["type"].(string); ok {
		msgType = t
	}

	replyType := inferReplyType(msgType)
	body := "Rejected"
	if reason != "" {
		body = "Rejected: " + reason
	}
	if err := sendReply(from, replyType, subject, body); err != nil {
		return err
	}

	if msgID != "" {
		return completeInboxMessage(msgID)
	}
	return nil
}

// showRejectDialog shows a text input dialog for rejection reason
func showRejectDialog(msg map[string]interface{}) {
	input := tview.NewInputField().
		SetLabel("Reason: ").
		SetFieldWidth(0).
		SetFieldTextColor(tcell.ColorBlack).
		SetFieldBackgroundColor(tcell.ColorWhite)

	input.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			reason := input.GetText()
			if err := rejectMessage(msg, reason); err != nil {
				updateStatus(fmt.Sprintf("[red]Error rejecting message: %v", err))
			} else {
				from := ""
				if f, ok := msg["from"].(string); ok {
					from = f
				}
				updateStatus(fmt.Sprintf("[green]Rejected message from %s", from))
				pages.RemovePage("reject-dialog")
				pages.RemovePage("message")
				if pages.HasPage("inbox") {
					pages.RemovePage("inbox")
					showInbox()
				}
				return
			}
		}
		pages.RemovePage("reject-dialog")
	})

	container := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(input, 1, 0, true)

	container.SetBorder(true).
		SetTitle(" Reject Message - Enter Reason (Enter to confirm, Esc to cancel) ").
		SetTitleAlign(tview.AlignLeft).
		SetBorderColor(tcell.ColorRed)

	pages.AddPage("reject-dialog", createModal(container, 70, 5), true, true)
}

func wrapBeadIDs(text string) string {
	// Match bead IDs like bd-5xz or bd-5xz.1
	re := regexp.MustCompile(`\bbd-[a-z0-9]+(?:\.[0-9]+)?\b`)
	return re.ReplaceAllStringFunc(text, func(id string) string {
		// Read bead to get title
		beadPath := filepath.Join("beads", id, "json")
		data, err := readFile(beadPath)
		if err != nil {
			return id // Return unwrapped if can't read
		}

		var bead map[string]interface{}
		if err := json.Unmarshal(data, &bead); err != nil {
			return id
		}

		title := ""
		if t, ok := bead["title"].(string); ok {
			title = t
		}

		return fmt.Sprintf("%s (%s)", id, title)
	})
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
	list.SetSelectedBackgroundColor(tcell.ColorWhite)
	list.SetSelectedTextColor(tcell.ColorBlack)

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
	form.SetFieldTextColor(tcell.ColorBlack)
	form.SetButtonTextColor(tcell.ColorBlack)

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

func showDaemonStatus() {
	data, err := readFile("daemon")
	if err != nil {
		updateStatus(fmt.Sprintf("[red]Error reading daemon status: %v", err))
		return
	}

	textView := tview.NewTextView().
		SetDynamicColors(true).
		SetText(string(data)).
		SetScrollable(true)

	textView.SetBorder(true).SetTitle(" Daemon Status ")
	textView.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Rune() == 'q' || event.Key() == tcell.KeyEscape {
			pages.RemovePage("daemon")
			return nil
		}
		return event
	})

	pages.AddPage("daemon", createModal(textView, 70, 20), true, true)
}

func showContextEditor() {
	sess := getSelectedSession()
	if sess == nil {
		updateStatus("[yellow]No session selected")
		return
	}

	// Read current context
	contextPath := filepath.Join(sess.ID, "context")
	contextData, err := readFile(contextPath)
	if err != nil {
		updateStatus(fmt.Sprintf("[red]Error reading context: %v", err))
		return
	}

	textArea := tview.NewTextArea().
		SetText(string(contextData), true)

	form := tview.NewForm().
		AddFormItem(textArea)
	form.SetFieldTextColor(tcell.ColorBlack)
	form.SetButtonTextColor(tcell.ColorBlack)

	form.AddButton("Save", func() {
		newContext := textArea.GetText()
		if err := writeFile(contextPath, []byte(newContext)); err != nil {
			updateStatus(fmt.Sprintf("[red]Error saving context: %v", err))
		} else {
			updateStatus(fmt.Sprintf("[green]Context saved for %s", sess.ID[:8]))
		}
		pages.RemovePage("context")
	})

	form.AddButton("Cancel", func() {
		pages.RemovePage("context")
	})

	form.SetBorder(true).
		SetTitle(fmt.Sprintf(" Edit Context - %s ", sess.ID[:8])).
		SetTitleAlign(tview.AlignLeft).
		SetBorderColor(tcell.ColorBlue)

	pages.AddPage("context", createModalDynamic(form, 4, 30), true, true)
}

func showInbox() {
	// Read inbox directory
	fid, err := fs.Open("user/inbox", plan9.OREAD)
	if err != nil {
		updateStatus(fmt.Sprintf("[red]Error opening inbox: %v", err))
		return
	}
	defer fid.Close()

	var messages []map[string]interface{}
	for {
		dirs, err := fid.Dirread()
		if err != nil || len(dirs) == 0 {
			break
		}
		for _, d := range dirs {
			if !strings.HasSuffix(d.Name, ".json") {
				continue
			}

			msgPath := filepath.Join("user/inbox", d.Name)
			msgData, err := readFile(msgPath)
			if err != nil {
				continue
			}

			var msg map[string]interface{}
			if err := json.Unmarshal(msgData, &msg); err != nil {
				continue
			}

			messages = append(messages, msg)
		}
	}

	list := tview.NewList().ShowSecondaryText(true)
	list.SetSelectedBackgroundColor(tcell.ColorWhite)
	list.SetSelectedTextColor(tcell.ColorBlack)

	for i, msg := range messages {
		idx := i
		from := ""
		if f, ok := msg["from"].(string); ok {
			from = f
		}
		subject := ""
		if s, ok := msg["subject"].(string); ok {
			subject = s
		}
		msgType := ""
		if t, ok := msg["type"].(string); ok {
			msgType = t
		}

		list.AddItem(
			fmt.Sprintf("(%s) %s", from, subject),
			msgType,
			0,
			func() {
				showMessage(messages[idx])
			},
		)
	}

	list.SetBorder(true).
		SetTitle(fmt.Sprintf(" Inbox (%d messages) | A:approve | X:reject | Q:close ", len(messages))).
		SetTitleAlign(tview.AlignLeft).
		SetBorderColor(tcell.ColorBlue)

	list.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Rune() == 'q' || event.Key() == tcell.KeyEscape {
			pages.RemovePage("inbox")
			return nil
		}
		if event.Rune() == 'a' {
			idx := list.GetCurrentItem()
			if idx >= 0 && idx < len(messages) {
				msg := messages[idx]
				if err := approveMessage(msg); err != nil {
					updateStatus(fmt.Sprintf("[red]Error approving message: %v", err))
				} else {
					from := ""
					if f, ok := msg["from"].(string); ok {
						from = f
					}
					updateStatus(fmt.Sprintf("[green]Approved message from %s", from))
					pages.RemovePage("inbox")
					showInbox()
				}
			}
			return nil
		}
		if event.Rune() == 'x' {
			idx := list.GetCurrentItem()
			if idx >= 0 && idx < len(messages) {
				showRejectDialog(messages[idx])
			}
			return nil
		}
		return event
	})

	pages.AddPage("inbox", createModalDynamic(list, 8, 16), true, true)
}

func showArchive() {
	// Read completed directory (archive)
	fid, err := fs.Open("user/completed", plan9.OREAD)
	if err != nil {
		updateStatus(fmt.Sprintf("[red]Error opening archive: %v", err))
		return
	}
	defer fid.Close()

	var messages []map[string]interface{}
	for {
		dirs, err := fid.Dirread()
		if err != nil || len(dirs) == 0 {
			break
		}
		for _, d := range dirs {
			if !strings.HasSuffix(d.Name, ".json") {
				continue
			}

			msgPath := filepath.Join("user/completed", d.Name)
			msgData, err := readFile(msgPath)
			if err != nil {
				continue
			}

			var msg map[string]interface{}
			if err := json.Unmarshal(msgData, &msg); err != nil {
				continue
			}

			messages = append(messages, msg)
		}
	}

	list := tview.NewList().ShowSecondaryText(true)
	list.SetSelectedBackgroundColor(tcell.ColorWhite)
	list.SetSelectedTextColor(tcell.ColorBlack)

	for i, msg := range messages {
		idx := i
		from := ""
		if f, ok := msg["from"].(string); ok {
			from = f
		}
		subject := ""
		if s, ok := msg["subject"].(string); ok {
			subject = s
		}
		msgType := ""
		if t, ok := msg["type"].(string); ok {
			msgType = t
		}

		list.AddItem(
			fmt.Sprintf("(%s) %s", from, subject),
			msgType,
			0,
			func() {
				showMessage(messages[idx])
			},
		)
	}

	list.SetBorder(true).
		SetTitle(fmt.Sprintf(" Archive (%d messages) ", len(messages))).
		SetTitleAlign(tview.AlignLeft).
		SetBorderColor(tcell.ColorBlue)

	list.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Rune() == 'q' || event.Key() == tcell.KeyEscape {
			pages.RemovePage("archive")
			return nil
		}
		return event
	})

	pages.AddPage("archive", createModalDynamic(list, 8, 16), true, true)
}

func attachToSession() {
	sess := getSelectedSession()
	if sess == nil {
		updateStatus("[yellow]No session selected")
		return
	}

	// Exit TUI and attach to tmux
	app.Suspend(func() {
		// Read tmux session name from session
		tmuxPath := filepath.Join(sess.ID, "tmux")
		tmuxData, err := readFile(tmuxPath)
		if err != nil {
			fmt.Printf("Error reading tmux session: %v\n", err)
			fmt.Println("Press Enter to continue...")
			fmt.Scanln()
			return
		}

		tmuxSession := strings.TrimSpace(string(tmuxData))
		if tmuxSession == "" {
			fmt.Println("No tmux session found for this agent")
			fmt.Println("Press Enter to continue...")
			fmt.Scanln()
			return
		}

		// Attach to tmux
		fmt.Printf("Attaching to tmux session: %s\n", tmuxSession)
		fmt.Printf("Press Ctrl-b d to detach\n\n")

		cmd := exec.Command("tmux", "attach-session", "-t", tmuxSession)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			fmt.Printf("Error attaching to tmux: %v\n", err)
			fmt.Println("Press Enter to continue...")
			fmt.Scanln()
		}
	})
}

func showMessage(msg map[string]interface{}) {
	body := ""
	if b, ok := msg["body"].(string); ok {
		body = b
	}
	from := ""
	if f, ok := msg["from"].(string); ok {
		from = f
	}
	subject := ""
	if s, ok := msg["subject"].(string); ok {
		subject = s
	}
	msgType := ""
	if t, ok := msg["type"].(string); ok {
		msgType = t
	}

	text := fmt.Sprintf("[yellow]From:[-] %s\n[yellow]Subject:[-] %s\n[yellow]Type:[-] %s\n\n%s", from, subject, msgType, body)

	textView := tview.NewTextView().
		SetDynamicColors(true).
		SetText(text).
		SetScrollable(true)

	// Show approve/reject keybindings for actionable request types
	needsApproval := msgType == "APPROVAL_REQUEST" || msgType == "REVIEW_REQUEST"
	title := " Message | Q:close"
	if needsApproval {
		title = " Message | A:approve | X:reject | Q:close"
	}

	textView.SetBorder(true).SetTitle(title)
	textView.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Rune() == 'q' || event.Key() == tcell.KeyEscape {
			pages.RemovePage("message")
			return nil
		}
		if event.Rune() == 'a' && needsApproval {
			if err := approveMessage(msg); err != nil {
				updateStatus(fmt.Sprintf("[red]Error approving message: %v", err))
			} else {
				updateStatus(fmt.Sprintf("[green]Approved message from %s", from))
				pages.RemovePage("message")
				if pages.HasPage("inbox") {
					pages.RemovePage("inbox")
					showInbox()
				}
			}
			return nil
		}
		if event.Rune() == 'x' && needsApproval {
			showRejectDialog(msg)
			return nil
		}
		return event
	})

	pages.AddPage("message", createModalDynamic(textView, 8, 16), true, true)
}

func showBeads() {
	showBeadsFiltered("")
}

func showBeadsFiltered(searchQuery string) {
	// Read ready beads
	beadsData, err := readFile("beads/ready")
	if err != nil {
		updateStatus(fmt.Sprintf("[red]Error reading beads: %v", err))
		return
	}

	var allBeads []map[string]interface{}
	if err := json.Unmarshal(beadsData, &allBeads); err != nil {
		updateStatus(fmt.Sprintf("[red]Error parsing beads: %v", err))
		return
	}

	// Filter beads if search query provided
	var beads []map[string]interface{}
	searchLower := strings.ToLower(searchQuery)
	for _, bead := range allBeads {
		if searchQuery == "" {
			beads = append(beads, bead)
		} else {
			title := ""
			if t, ok := bead["title"].(string); ok {
				title = strings.ToLower(t)
			}
			if strings.Contains(title, searchLower) {
				beads = append(beads, bead)
			}
		}
	}

	list := tview.NewList().ShowSecondaryText(true)
	list.SetSelectedBackgroundColor(tcell.ColorWhite)
	list.SetSelectedTextColor(tcell.ColorBlack)

	for i, bead := range beads {
		idx := i
		id := ""
		if bid, ok := bead["id"].(string); ok {
			id = bid
		}
		title := ""
		if t, ok := bead["title"].(string); ok {
			title = t
		}
		status := ""
		if s, ok := bead["status"].(string); ok {
			status = s
		}

		list.AddItem(
			fmt.Sprintf("(%s) %s", id, title),
			status,
			0,
			func() {
				showBead(beads[idx])
			},
		)
	}

	// Get selected session info for display
	sess := getSelectedSession()
	sessionInfo := ""
	if sess != nil {
		sessionInfo = fmt.Sprintf("(%s) ", sess.ID[:8])
	}

	titleStr := fmt.Sprintf(" %sReady Beads (%d) | N:new | C:subtask | S:search | W:assign | R:refresh | Q:quit ", sessionInfo, len(beads))
	if searchQuery != "" {
		titleStr = fmt.Sprintf(" %sSearch: '%s' (%d) | N:new | C:subtask | S:search | W:assign | R:refresh | Q:quit ", sessionInfo, searchQuery, len(beads))
	}

	list.SetBorder(true).
		SetTitle(titleStr).
		SetTitleAlign(tview.AlignLeft).
		SetBorderColor(tcell.ColorBlue)

	list.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Rune() == 'q' || event.Key() == tcell.KeyEscape {
			pages.RemovePage("beads")
			return nil
		}
		if event.Rune() == 'w' {
			// Get selected bead
			idx := list.GetCurrentItem()
			if idx >= 0 && idx < len(beads) {
				showAssignBeadConfirm(beads[idx])
			}
			return nil
		}
		if event.Rune() == 'n' {
			showNewBeadDialog()
			return nil
		}
		if event.Rune() == 'c' {
			// Get selected bead
			idx := list.GetCurrentItem()
			if idx >= 0 && idx < len(beads) {
				parentID := ""
				if bid, ok := beads[idx]["id"].(string); ok {
					parentID = bid
				}
				if parentID != "" {
					showNewSubtaskDialog(parentID)
				}
			}
			return nil
		}
		if event.Rune() == 's' {
			showBeadSearchDialog()
			return nil
		}
		if event.Rune() == 'r' {
			pages.RemovePage("beads")
			showBeads()
			return nil
		}
		return event
	})

	pages.AddPage("beads", createModalDynamic(list, 8, 16), true, true)
}

func showBeadSearchDialog() {
	input := tview.NewInputField().
		SetLabel("Search: ").
		SetFieldWidth(0).
		SetFieldTextColor(tcell.ColorBlack).
		SetFieldBackgroundColor(tcell.ColorWhite)

	input.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			query := input.GetText()
			pages.RemovePage("bead-search")
			pages.RemovePage("beads")
			showBeadsFiltered(query)
		} else {
			pages.RemovePage("bead-search")
		}
	})

	container := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(input, 1, 0, true)

	container.SetBorder(true).
		SetTitle(" Search Beads by Title ").
		SetTitleAlign(tview.AlignLeft).
		SetBorderColor(tcell.ColorBlue)

	pages.AddPage("bead-search", createModal(container, 50, 5), true, true)
}

func showBead(bead map[string]interface{}) {
	id := ""
	if bid, ok := bead["id"].(string); ok {
		id = bid
	}
	title := ""
	if t, ok := bead["title"].(string); ok {
		title = t
	}
	description := ""
	if d, ok := bead["description"].(string); ok {
		description = d
	}
	status := ""
	if s, ok := bead["status"].(string); ok {
		status = s
	}

	text := fmt.Sprintf("[yellow]ID:[-] %s\n[yellow]Title:[-] %s\n[yellow]Status:[-] %s\n\n%s", id, title, status, description)

	textView := tview.NewTextView().
		SetDynamicColors(true).
		SetText(text).
		SetScrollable(true)

	textView.SetBorder(true).SetTitle(" Bead Details | E:edit | N:new subtask | Q:quit ")
	textView.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Rune() == 'q' || event.Key() == tcell.KeyEscape {
			pages.RemovePage("bead")
			return nil
		}
		if event.Rune() == 'n' {
			showNewSubtaskDialog(id)
			return nil
		}
		if event.Rune() == 'e' {
			showEditBeadDialog(bead)
			return nil
		}
		return event
	})

	pages.AddPage("bead", createModalDynamic(textView, 8, 16), true, true)
}

func showNewBeadDialog() {
	titleInput := tview.NewInputField().
		SetLabel("Title: ").
		SetFieldWidth(0).
		SetFieldTextColor(tcell.ColorBlack).
		SetFieldBackgroundColor(tcell.ColorWhite)

	descArea := tview.NewTextArea().
		SetPlaceholder("Description (optional)...")

	form := tview.NewForm().
		AddFormItem(titleInput).
		AddFormItem(descArea)
	form.SetFieldTextColor(tcell.ColorBlack)
	form.SetButtonTextColor(tcell.ColorBlack)

	form.AddButton("Create", func() {
		title := titleInput.GetText()
		desc := descArea.GetText()

		if title == "" {
			updateStatus("[red]Title cannot be empty")
			return
		}

		// Escape single quotes in title and description
		title = strings.ReplaceAll(title, "'", "\\'")
		desc = strings.ReplaceAll(desc, "'", "\\'")

		cmd := fmt.Sprintf("new '%s' '%s'", title, desc)
		if err := writeFile("beads/ctl", []byte(cmd)); err != nil {
			updateStatus(fmt.Sprintf("[red]Error creating bead: %v", err))
		} else {
			updateStatus(fmt.Sprintf("[green]Created bead: %s", title))
			pages.RemovePage("new-bead")
			// Refresh beads view if it's open
			if pages.HasPage("beads") {
				pages.RemovePage("beads")
				showBeads()
			}
		}
	})

	form.AddButton("Cancel", func() {
		pages.RemovePage("new-bead")
	})

	form.SetBorder(true).
		SetTitle(" Create New Bead ").
		SetTitleAlign(tview.AlignLeft).
		SetBorderColor(tcell.ColorBlue)

	pages.AddPage("new-bead", createModal(form, 80, 15), true, true)
}

func showNewSubtaskDialog(parentID string) {
	titleInput := tview.NewInputField().
		SetLabel("Title: ").
		SetFieldWidth(0).
		SetFieldTextColor(tcell.ColorBlack).
		SetFieldBackgroundColor(tcell.ColorWhite)

	descArea := tview.NewTextArea().
		SetPlaceholder("Description (optional)...")

	form := tview.NewForm().
		AddFormItem(titleInput).
		AddFormItem(descArea)
	form.SetFieldTextColor(tcell.ColorBlack)
	form.SetButtonTextColor(tcell.ColorBlack)

	form.AddButton("Create", func() {
		title := titleInput.GetText()
		desc := descArea.GetText()

		if title == "" {
			updateStatus("[red]Title cannot be empty")
			return
		}

		// Escape single quotes in title and description
		title = strings.ReplaceAll(title, "'", "\\'")
		desc = strings.ReplaceAll(desc, "'", "\\'")

		cmd := fmt.Sprintf("new '%s' '%s' %s", title, desc, parentID)
		if err := writeFile("beads/ctl", []byte(cmd)); err != nil {
			updateStatus(fmt.Sprintf("[red]Error creating subtask: %v", err))
		} else {
			updateStatus(fmt.Sprintf("[green]Created subtask under %s: %s", parentID, title))
			pages.RemovePage("new-subtask")
			pages.RemovePage("bead")
		}
	})

	form.AddButton("Cancel", func() {
		pages.RemovePage("new-subtask")
	})

	form.SetBorder(true).
		SetTitle(fmt.Sprintf(" Create Subtask (parent: %s) ", parentID)).
		SetTitleAlign(tview.AlignLeft).
		SetBorderColor(tcell.ColorBlue)

	pages.AddPage("new-subtask", createModal(form, 80, 15), true, true)
}

func showEditBeadDialog(bead map[string]interface{}) {
	id := ""
	if bid, ok := bead["id"].(string); ok {
		id = bid
	}
	title := ""
	if t, ok := bead["title"].(string); ok {
		title = t
	}
	description := ""
	if d, ok := bead["description"].(string); ok {
		description = d
	}
	status := ""
	if s, ok := bead["status"].(string); ok {
		status = s
	}
	priority := 0
	if p, ok := bead["priority"].(float64); ok {
		priority = int(p)
	}

	titleInput := tview.NewInputField().
		SetLabel("Title: ").
		SetText(title).
		SetFieldWidth(0).
		SetFieldTextColor(tcell.ColorBlack).
		SetFieldBackgroundColor(tcell.ColorWhite)

	descArea := tview.NewTextArea().
		SetText(description, false)

	statusInput := tview.NewInputField().
		SetLabel("Status: ").
		SetText(status).
		SetFieldWidth(0).
		SetFieldTextColor(tcell.ColorBlack).
		SetFieldBackgroundColor(tcell.ColorWhite)

	priorityInput := tview.NewInputField().
		SetLabel("Priority: ").
		SetText(fmt.Sprintf("%d", priority)).
		SetFieldWidth(0).
		SetFieldTextColor(tcell.ColorBlack).
		SetFieldBackgroundColor(tcell.ColorWhite)

	form := tview.NewForm().
		AddFormItem(titleInput).
		AddFormItem(descArea).
		AddFormItem(statusInput).
		AddFormItem(priorityInput)
	form.SetFieldTextColor(tcell.ColorBlack)
	form.SetButtonTextColor(tcell.ColorBlack)

	form.AddButton("Save", func() {
		newTitle := titleInput.GetText()
		newDesc := descArea.GetText()
		newStatus := statusInput.GetText()
		newPriorityStr := priorityInput.GetText()

		if newTitle == "" {
			updateStatus("[red]Title cannot be empty")
			return
		}

		// Update title if changed
		if newTitle != title {
			cmd := fmt.Sprintf("update %s title '%s'", id, strings.ReplaceAll(newTitle, "'", "\\'"))
			if err := writeFile("beads/ctl", []byte(cmd)); err != nil {
				updateStatus(fmt.Sprintf("[red]Error updating title: %v", err))
				return
			}
		}

		// Update description if changed
		if newDesc != description {
			cmd := fmt.Sprintf("update %s description '%s'", id, strings.ReplaceAll(newDesc, "'", "\\'"))
			if err := writeFile("beads/ctl", []byte(cmd)); err != nil {
				updateStatus(fmt.Sprintf("[red]Error updating description: %v", err))
				return
			}
		}

		// Update status if changed
		if newStatus != status {
			cmd := fmt.Sprintf("update %s status %s", id, newStatus)
			if err := writeFile("beads/ctl", []byte(cmd)); err != nil {
				updateStatus(fmt.Sprintf("[red]Error updating status: %v", err))
				return
			}
		}

		// Update priority if changed
		if newPriorityStr != fmt.Sprintf("%d", priority) {
			cmd := fmt.Sprintf("update %s priority %s", id, newPriorityStr)
			if err := writeFile("beads/ctl", []byte(cmd)); err != nil {
				updateStatus(fmt.Sprintf("[red]Error updating priority: %v", err))
				return
			}
		}

		updateStatus(fmt.Sprintf("[green]Updated bead %s", id))
		pages.RemovePage("edit-bead")
		pages.RemovePage("bead")
		// Refresh beads view if it's open
		if pages.HasPage("beads") {
			pages.RemovePage("beads")
			showBeads()
		}
	})

	form.AddButton("Cancel", func() {
		pages.RemovePage("edit-bead")
	})

	form.SetBorder(true).
		SetTitle(fmt.Sprintf(" Edit Bead: %s ", id)).
		SetTitleAlign(tview.AlignLeft).
		SetBorderColor(tcell.ColorBlue)

	pages.AddPage("edit-bead", createModal(form, 80, 20), true, true)
}

func showAssignBeadConfirm(bead map[string]interface{}) {
	// Get currently selected session
	sess := getSelectedSession()
	if sess == nil {
		updateStatus("[yellow]No session selected")
		return
	}

	beadID := ""
	if bid, ok := bead["id"].(string); ok {
		beadID = bid
	}
	beadTitle := ""
	if t, ok := bead["title"].(string); ok {
		beadTitle = t
	}

	if beadID == "" {
		updateStatus("[red]Invalid bead")
		return
	}

	message := fmt.Sprintf("Agent %s will work on %s (%s).\n\nContinue?", sess.ID[:8], beadID, beadTitle)

	textView := tview.NewTextView().
		SetText(message).
		SetTextAlign(tview.AlignCenter)

	form := tview.NewForm()
	form.SetButtonTextColor(tcell.ColorBlack)
	form.AddButton("Yes", func() {
		// Send prompt to work on the bead
		prompt := fmt.Sprintf("Work on bead %s", beadID)
		if err := sendPrompt(sess.ID, prompt); err != nil {
			updateStatus(fmt.Sprintf("[red]Error sending prompt: %v", err))
		} else {
			updateStatus(fmt.Sprintf("[green]Assigned bead %s to %s", beadID, sess.ID[:8]))
		}
		pages.RemovePage("assign-confirm")
	})

	form.AddButton("No", func() {
		pages.RemovePage("assign-confirm")
	})

	layout := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(textView, 0, 1, false).
		AddItem(form, 3, 0, true)

	layout.SetBorder(true).
		SetTitle(" Assign Bead ").
		SetTitleAlign(tview.AlignLeft).
		SetBorderColor(tcell.ColorBlue)

	pages.AddPage("assign-confirm", createModal(layout, 60, 10), true, true)
}

func showHelp() {
	helpText := `
[yellow]AnviLLM TUI - Keyboard Shortcuts[-]

[yellow]Session Management:[-]
  s       Start new session (shows backend menu)
  T       Stop selected session
  R       Restart selected session
  K       Kill selected session
  A       Set session alias
  t       Attach to session tmux

[yellow]Interaction:[-]
  space   Send prompt to session
  c       Edit session context
  i       View inbox
  a       View archive
  b       View/claim beads (tasks)
  r       Refresh session list

[yellow]Inbox:[-]
  a       Approve selected message (sends APPROVAL/REVIEW_RESPONSE)
  x       Reject selected message (prompts for reason)
  Enter   View message details
  q/Esc   Close inbox

[yellow]Navigation:[-]
  ↑/↓     Select session (arrow keys)
  j/n     Select next session (down)
  k/p     Select previous session (up)
  C-n/C-p Select session (emacs-style)

[yellow]Other:[-]
  d       Daemon status
  ?       Show this help
  q       Quit (or ESC in dialogs)

[yellow]Beads:[-]
  b       View ready beads
  n       Create new bead
  c       Create subtask of SELECTED bead
  e       Edit bead (in bead detail view)
  s       Search beads by title (blank = show all)
  w       Assign SELECTED bead to CURRENTLY SELECTED SESSION (sends prompt)
  r       Refresh / clear search

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

func createModalDynamic(p tview.Primitive, widthProportion, heightProportion int) tview.Primitive {
	return tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(p, 0, heightProportion, true).
			AddItem(nil, 0, 1, false), 0, widthProportion, true).
		AddItem(nil, 0, 1, false)
}
