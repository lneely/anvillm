package main

import (
	"bufio"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"9fans.net/go/acme"
	_ "github.com/mattn/go-sqlite3"
)

var (
	ansiRegex      = regexp.MustCompile(`\x1b\[[0-9;]*m`)
	conversationID string
	cwd            string
	dbPath         string
)

func stripAnsi(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}

func loadEnvFiles() {
	for _, file := range []string{"jira.env.gpg", "jenkins.env.gpg"} {
		cmd := exec.Command("gpg", "--decrypt", filepath.Join(os.Getenv("HOME"), "env", file))
		cmd.Stderr = nil
		if output, err := cmd.Output(); err == nil {
			for _, line := range strings.Split(string(output), "\n") {
				if strings.HasPrefix(line, "export ") {
					if parts := strings.SplitN(line[7:], "=", 2); len(parts) == 2 {
						os.Setenv(parts[0], parts[1])
					}
				}
			}
		}
	}
}

// touchConversation updates the timestamp to make our conversation the most recent
func touchConversation() error {
	if conversationID == "" {
		return nil
	}
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec("UPDATE conversations_v2 SET updated_at = ? WHERE key = ? AND conversation_id = ?",
		time.Now().UnixMilli(), cwd, conversationID)
	return err
}

// getLatestConversationID retrieves the most recent conversation ID for cwd
func getLatestConversationID() (string, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return "", err
	}
	defer db.Close()
	var id string
	err = db.QueryRow("SELECT conversation_id FROM conversations_v2 WHERE key = ? ORDER BY updated_at DESC LIMIT 1", cwd).Scan(&id)
	return id, err
}

func runPrompt(outputWin *acme.Win, prompt []byte) {
	outputWin.Addr("$")
	outputWin.Write("data", []byte("USER:\n"))
	outputWin.Write("data", prompt)
	outputWin.Write("data", []byte("\n\nQ:\n"))

	args := []string{"run-session", "--", "kiro-cli", "chat", "--no-interactive", "-a"}
	if conversationID != "" {
		touchConversation()
		args = append(args, "--resume")
	}

	cmd := exec.Command("superpowers", args...)
	cmd.Stdin = strings.NewReader(string(prompt))
	cmd.Env = append(os.Environ(), "NO_COLOR=1", "TERM=xterm", "COLUMNS=999")

	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	cmd.Start()

	var hasError, hasOutput bool
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := stripAnsi(scanner.Text())
			if !hasError {
				outputWin.Write("data", []byte("\n\nDEBUG:\n"))
				hasError = true
			}
			outputWin.Write("data", []byte(line+"\n"))
		}
	}()

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := stripAnsi(scanner.Text())
		if hasError && !hasOutput {
			outputWin.Write("data", []byte("\nQ:\n"))
			hasOutput = true
		}
		outputWin.Write("data", []byte(line+"\n"))
	}

	cmd.Wait()
	outputWin.Write("data", []byte("\n"+strings.Repeat("=", 20)+"\n\n"))
	outputWin.Ctl("clean")
	outputWin.Ctl("addr=$")
	outputWin.Ctl("dot=addr")
	outputWin.Ctl("show")

	// Capture conversation ID after first run
	if conversationID == "" {
		if id, err := getLatestConversationID(); err == nil {
			conversationID = id
		}
	}
}

func main() {
	cwd, _ = os.Getwd()
	dbPath = filepath.Join(os.Getenv("HOME"), ".local/share/kiro-cli/data.sqlite3")
	loadEnvFiles()

	// Create prompt window
	promptWin, _ := acme.New()
	promptWin.Name(cwd + "/+Q-Prompt")
	promptWin.Write("tag", []byte(" Send"))
	promptWin.Ctl("clean")

	// Create output window
	outputWin, _ := acme.New()
	outputWin.Name(cwd + "/+Q-Output")
	outputWin.Ctl("clean")

	// Event loop - exits when window is deleted
	for evt := range promptWin.EventChan() {
		if evt.C2 == 'x' || evt.C2 == 'X' { // execute
			cmd := strings.TrimSpace(string(evt.Text))
			if cmd == "Send" {
				promptWin.Addr(",")
				prompt, _ := promptWin.ReadAll("data")
				if len(strings.TrimSpace(string(prompt))) > 0 {
					promptWin.Addr(",")
					promptWin.Write("data", nil)
					go runPrompt(outputWin, prompt)
				}
			} else if cmd == "Del" {
				promptWin.Ctl("delete")
				break
			} else {
				promptWin.WriteEvent(evt)
			}
		} else {
			promptWin.WriteEvent(evt)
		}
	}
}
