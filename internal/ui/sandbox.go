package ui

import (
	"anvillm/internal/sandbox"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"9fans.net/go/acme"
)

// OpenSandboxWindow opens the sandbox configuration window
func OpenSandboxWindow() error {
	w, err := acme.New()
	if err != nil {
		return err
	}

	w.Name("/AnviLLM/Sandbox")
	w.Write("tag", []byte("Get Edit Reload Status "))

	go handleSandboxWindow(w)
	return nil
}

func handleSandboxWindow(w *acme.Win) {
	defer w.CloseFiles()

	// Load and display current config
	refreshSandboxWindow(w)

	for e := range w.EventChan() {
		switch e.C2 {
		case 'x', 'X':
			cmd := string(e.Text)

			switch cmd {
			case "Get":
				refreshSandboxWindow(w)
			case "Edit":
				openConfigInEditor()
			case "Reload":
				// Config is edited externally, just reload
				refreshSandboxWindow(w)
				fmt.Println("Reloaded config from disk. Changes apply to new sessions only.")
			case "Status":
				showStatus(w)
			default:
				w.WriteEvent(e)
			}
		default:
			w.WriteEvent(e)
		}
	}
}

func refreshSandboxWindow(w *acme.Win) {
	cfg, err := sandbox.Load()
	if err != nil {
		w.Addr(",")
		w.Write("data", []byte(fmt.Sprintf("Error loading config: %v\n", err)))
		w.Ctl("clean")
		return
	}

	var buf strings.Builder

	buf.WriteString("AnviLLM Sandbox Configuration\n")
	buf.WriteString(strings.Repeat("=", 60) + "\n\n")

	// Summary
	buf.WriteString(sandbox.BuildSummary(cfg))
	buf.WriteString("\n\n")

	// Instructions
	buf.WriteString(strings.Repeat("-", 60) + "\n")
	buf.WriteString("Commands:\n")
	buf.WriteString("  Edit   - Open config file in $EDITOR or Acme\n")
	buf.WriteString("  Reload - Reload config from disk\n")
	buf.WriteString("  Status - Check landrun availability\n")
	buf.WriteString("  Get    - Refresh this window\n\n")

	buf.WriteString("Config file: " + sandbox.ConfigPath() + "\n")
	buf.WriteString("Note: Changes apply to NEW sessions only.\n")
	buf.WriteString("      Active sessions are NOT affected.\n")

	w.Addr(",")
	w.Write("data", []byte(buf.String()))
	w.Ctl("clean")
}

func openConfigInEditor() {
	path := sandbox.ConfigPath()

	// Try to open in Acme via 9p
	cmd := exec.Command("9p", "write", "acme/new/ctl")
	stdin, err := cmd.StdinPipe()
	if err == nil {
		cmd.Start()
		fmt.Fprintf(stdin, "name %s\n", path)
		fmt.Fprintf(stdin, "get\n")
		stdin.Close()
		cmd.Wait()
		return
	}

	// Fallback: use environment EDITOR
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	cmd = exec.Command(editor, path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Run()
}

func showStatus(w *acme.Win) {
	var buf strings.Builder

	buf.WriteString("\n")
	buf.WriteString(strings.Repeat("-", 60) + "\n")
	buf.WriteString("Landrun Status:\n")
	buf.WriteString(strings.Repeat("-", 60) + "\n")

	if sandbox.IsAvailable() {
		buf.WriteString("Status: AVAILABLE\n")
		version := sandbox.Version()
		if version != "" {
			buf.WriteString("Version: " + version + "\n")
		}
		if path, err := exec.LookPath("landrun"); err == nil {
			buf.WriteString("Location: " + path + "\n")
		}
	} else {
		buf.WriteString("Status: NOT AVAILABLE\n")
		buf.WriteString("\n")
		buf.WriteString("Install landrun:\n")
		buf.WriteString("  go install github.com/landlock-lsm/landrun@latest\n")
		buf.WriteString("\n")
		buf.WriteString("Or download from:\n")
		buf.WriteString("  https://github.com/landlock-lsm/landrun\n")
	}

	w.Addr("$")
	w.Write("data", []byte(buf.String()))
}
