// anvilwebgw - AnviLLM HTTP Gateway Service
//
// Standalone HTTP gateway that exposes the 9P filesystem served by anvilsrv
// as an HTTP API. Supports start, stop, fgstart, and status subcommands.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"9fans.net/go/plan9"
	"9fans.net/go/plan9/client"
)

const terminalCommand = "foot"

var (
	addr      = flag.String("addr", ":8081", "HTTP gateway server address")
	namespace = flag.String("ns", os.Getenv("NAMESPACE"), "9P namespace path")
)

func getPidFilePath() string {
	ns := client.Namespace()
	if ns == "" {
		return filepath.Join("/tmp", "anvilwebgw.pid")
	}
	return filepath.Join(ns, "anvilwebgw.pid")
}

func main() {
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		usage()
		os.Exit(1)
	}

	switch args[0] {
	case "start":
		startCmd(true)
	case "fgstart":
		startCmd(false)
	case "stop":
		stopCmd()
	case "status":
		statusCmd()
	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: %s [flags] {start|fgstart|stop|status}\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "Commands:\n")
	fmt.Fprintf(os.Stderr, "  start   - Start the gateway daemon (daemonized)\n")
	fmt.Fprintf(os.Stderr, "  fgstart - Start in foreground (for debugging)\n")
	fmt.Fprintf(os.Stderr, "  stop    - Stop the running daemon\n")
	fmt.Fprintf(os.Stderr, "  status  - Check daemon status\n")
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "Flags:\n")
	flag.PrintDefaults()
}

func startCmd(daemonize bool) {
	pidFile := getPidFilePath()

	// Check if already running before daemonizing
	if existingPid := readPidFile(); existingPid != 0 {
		if isProcessRunning(existingPid) {
			log.Fatalf("anvilwebgw already running (PID %d)", existingPid)
		}
		// Stale PID file
		os.Remove(pidFile)
	}

	// Daemonize if requested
	if daemonize {
		if os.Getenv("ANVILWEBGW_DAEMON") != "1" {
			cmd, err := os.Executable()
			if err != nil {
				log.Fatalf("Failed to get executable path: %v", err)
			}
			// Reconstruct args: flags + fgstart
			fwdArgs := []string{"fgstart"}
			flag.Visit(func(f *flag.Flag) {
				fwdArgs = append([]string{"-" + f.Name + "=" + f.Value.String()}, fwdArgs...)
			})

			attr := &os.ProcAttr{
				Dir: "/",
				Env: append(os.Environ(), "ANVILWEBGW_DAEMON=1"),
				Files: []*os.File{nil, nil, nil},
			}

			proc, err := os.StartProcess(cmd, append([]string{cmd}, fwdArgs...), attr)
			if err != nil {
				log.Fatalf("Failed to daemonize: %v", err)
			}
			proc.Release()

			for i := 0; i < 10; i++ {
				if readPidFile() != 0 {
					fmt.Fprintf(os.Stderr, "anvilwebgw started successfully\n")
					return
				}
				time.Sleep(100 * time.Millisecond)
			}
			fmt.Fprintf(os.Stderr, "anvilwebgw started (daemon mode)\n")
			return
		}
	}

	// We're the daemon (or running in foreground)
	pid := os.Getpid()
	f, err := os.OpenFile(pidFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		if existingPid := readPidFile(); existingPid != 0 {
			if isProcessRunning(existingPid) {
				log.Fatalf("anvilwebgw already running (PID %d)", existingPid)
			}
			os.Remove(pidFile)
			f, err = os.OpenFile(pidFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
			if err != nil {
				log.Fatalf("Failed to create PID file: %v", err)
			}
		} else {
			log.Fatalf("Failed to create PID file: %v", err)
		}
	}
	if _, err := fmt.Fprintf(f, "%d\n", pid); err != nil {
		f.Close()
		os.Remove(pidFile)
		log.Fatalf("Failed to write PID file: %v", err)
	}
	f.Close()
	defer os.Remove(pidFile)

	if *namespace == "" {
		*namespace = fmt.Sprintf("/tmp/ns.%s.:0", os.Getenv("USER"))
	}

	// Register HTTP handlers
	mux := http.NewServeMux()
	mux.HandleFunc("/api/sessions", handleSessions)
	mux.HandleFunc("/api/session/", handleSession)
	mux.HandleFunc("/api/inbox", handleInbox)
	mux.HandleFunc("/api/inbox/count", handleInboxCount)
	mux.HandleFunc("/api/inbox/reply", handleInboxReply)
	mux.HandleFunc("/api/inbox/archive", handleInboxArchive)
	mux.HandleFunc("/api/archive", handleArchive)
	mux.HandleFunc("/api/beads", handleBeads)
	mux.HandleFunc("/api/beads/ready", handleBeadsReady)
	mux.HandleFunc("/api/beads/ctl", handleBeadsCtl)
	mux.HandleFunc("/api/beads/", handleBeadDetail)
	mux.HandleFunc("/api/daemon/status", handleDaemonStatus)

	server := &http.Server{
		Addr:    *addr,
		Handler: mux,
	}

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		log.Printf("Received signal %v, shutting down", sig)
		server.Close()
	}()

	log.Printf("anvilwebgw started successfully")
	log.Printf("HTTP gateway listening on %s", *addr)
	log.Printf("Using 9P namespace: %s", *namespace)

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func stopCmd() {
	pidFile := getPidFilePath()
	pid := readPidFile()
	if pid == 0 {
		fmt.Fprintf(os.Stderr, "anvilwebgw is not running\n")
		os.Exit(1)
	}

	if !isProcessRunning(pid) {
		fmt.Fprintf(os.Stderr, "anvilwebgw is not running (stale PID file)\n")
		os.Remove(pidFile)
		os.Exit(1)
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to find process: %v\n", err)
		os.Exit(1)
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to send SIGTERM: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Sent SIGTERM to anvilwebgw (PID %d)\n", pid)
}

func statusCmd() {
	pid := readPidFile()
	if pid == 0 {
		fmt.Println("anvilwebgw is not running")
		os.Exit(1)
	}

	if !isProcessRunning(pid) {
		fmt.Println("anvilwebgw is not running (stale PID file)")
		os.Exit(1)
	}

	fmt.Printf("anvilwebgw is running (PID %d)\n", pid)
}

func readPidFile() int {
	data, err := os.ReadFile(getPidFilePath())
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	return pid
}

func isProcessRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}

// connect opens a 9P connection to anvilsrv
func connect() (*client.Fsys, error) {
	ns := *namespace
	if ns == "" {
		ns = fmt.Sprintf("/tmp/ns.%s.:0", os.Getenv("USER"))
	}
	return client.Mount("unix", filepath.Join(ns, "agent"))
}

// --- Session types ---

type Session struct {
	ID      string `json:"id"`
	Alias   string `json:"alias"`
	State   string `json:"state"`
	PID     string `json:"pid"`
	CWD     string `json:"cwd"`
	Backend string `json:"backend"`
}

// --- Session handlers ---

func handleSessions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case "GET":
		listSessions(w, r)
	case "POST":
		createSession(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func listSessions(w http.ResponseWriter, r *http.Request) {
	fs, err := connect()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer fs.Close()

	fid, err := fs.Open("list", plan9.OREAD)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer fid.Close()

	data, err := io.ReadAll(fid)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var sessions []Session
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		sess := Session{
			ID:    fields[0],
			Alias: fields[1],
			State: fields[2],
			PID:   fields[3],
			CWD:   fields[4],
		}

		if backendFid, err := fs.Open(filepath.Join(sess.ID, "backend"), plan9.OREAD); err == nil {
			if backendData, err := io.ReadAll(backendFid); err == nil {
				sess.Backend = strings.TrimSpace(string(backendData))
			}
			backendFid.Close()
		}

		sessions = append(sessions, sess)
	}

	json.NewEncoder(w).Encode(sessions)
}

func createSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Backend string `json:"backend"`
		CWD     string `json:"cwd"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Backend != "kiro-cli" && req.Backend != "claude" {
		http.Error(w, "Invalid backend", http.StatusBadRequest)
		return
	}

	if !filepath.IsAbs(req.CWD) {
		http.Error(w, "Path must be absolute", http.StatusBadRequest)
		return
	}
	if _, err := os.Stat(req.CWD); err != nil {
		http.Error(w, "Path does not exist", http.StatusBadRequest)
		return
	}

	fs, err := connect()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer fs.Close()

	fid, err := fs.Open("ctl", plan9.OWRITE)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer fid.Close()

	if _, err := fid.Write([]byte(fmt.Sprintf("new %s %s", req.Backend, req.CWD))); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func handleSession(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/session/"), "/")
	if len(parts) < 1 {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	id := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	switch r.Method {
	case "GET":
		if action == "" {
			getSession(w, r, id)
		} else {
			getSessionFile(w, r, id, action)
		}
	case "POST":
		switch action {
		case "prompt":
			sendPrompt(w, r, id)
		case "prompt-direct":
			sendPromptDirect(w, r, id)
		case "ctl":
			controlSession(w, r, id)
		default:
			http.Error(w, "Invalid action", http.StatusBadRequest)
		}
	case "PUT":
		if action == "alias" || action == "context" {
			updateSessionFile(w, r, id, action)
		} else {
			http.Error(w, "Invalid action", http.StatusBadRequest)
		}
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func getSession(w http.ResponseWriter, r *http.Request, id string) {
	fs, err := connect()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer fs.Close()

	sess := Session{ID: id}

	readField := func(name string) string {
		fid, err := fs.Open(filepath.Join(id, name), plan9.OREAD)
		if err != nil {
			return ""
		}
		defer fid.Close()
		data, _ := io.ReadAll(fid)
		return strings.TrimSpace(string(data))
	}

	sess.State = readField("state")
	sess.Alias = readField("alias")
	sess.Backend = readField("backend")
	sess.PID = readField("pid")
	sess.CWD = readField("cwd")

	json.NewEncoder(w).Encode(sess)
}

func getSessionTmux(id string) string {
	fs, err := connect()
	if err != nil {
		return ""
	}
	defer fs.Close()

	fid, err := fs.Open(filepath.Join(id, "tmux"), plan9.OREAD)
	if err != nil {
		return ""
	}
	defer fid.Close()

	data, _ := io.ReadAll(fid)
	return strings.TrimSpace(string(data))
}

func getSessionFile(w http.ResponseWriter, r *http.Request, id, file string) {
	fs, err := connect()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer fs.Close()

	fid, err := fs.Open(filepath.Join(id, file), plan9.OREAD)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer fid.Close()

	data, err := io.ReadAll(fid)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"content": string(data)})
}

func sendPrompt(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Prompt string `json:"prompt"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(req.Prompt) == "" {
		http.Error(w, "Prompt cannot be empty", http.StatusBadRequest)
		return
	}

	fs, err := connect()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer fs.Close()

	msg := map[string]interface{}{
		"to":      id,
		"type":    "PROMPT_REQUEST",
		"subject": "User prompt",
		"body":    req.Prompt,
	}
	msgJSON, err := json.Marshal(msg)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to marshal message: %v", err), http.StatusInternalServerError)
		return
	}

	fid, err := fs.Open("user/mail", plan9.OWRITE)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to open mail file: %v", err), http.StatusInternalServerError)
		return
	}
	defer fid.Close()

	if _, err := fid.Write(msgJSON); err != nil {
		http.Error(w, fmt.Sprintf("failed to write message: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func sendPromptDirect(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Prompt string `json:"prompt"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(req.Prompt) == "" {
		http.Error(w, "Prompt cannot be empty", http.StatusBadRequest)
		return
	}

	fs, err := connect()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer fs.Close()

	fid, err := fs.Open(filepath.Join(id, "in"), plan9.OWRITE)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer fid.Close()

	if _, err := fid.Write([]byte(req.Prompt)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func controlSession(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Command string `json:"command"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	validCmds := map[string]bool{"stop": true, "restart": true, "kill": true, "refresh": true, "attach": true}
	if !validCmds[req.Command] {
		http.Error(w, "Invalid command", http.StatusBadRequest)
		return
	}

	if req.Command == "attach" {
		tmuxTarget := getSessionTmux(id)
		if tmuxTarget == "" {
			http.Error(w, "Session does not support attach", http.StatusBadRequest)
			return
		}
		cmd := exec.Command(terminalCommand, "-e", "tmux", "attach", "-t", tmuxTarget)
		if err := cmd.Start(); err != nil {
			http.Error(w, "Failed to launch terminal: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		return
	}

	fs, err := connect()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer fs.Close()

	fid, err := fs.Open(filepath.Join(id, "ctl"), plan9.OWRITE)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer fid.Close()

	if _, err := fid.Write([]byte(req.Command)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func updateSessionFile(w http.ResponseWriter, r *http.Request, id, file string) {
	var req struct {
		Content string `json:"content"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	fs, err := connect()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer fs.Close()

	fid, err := fs.Open(filepath.Join(id, file), plan9.OWRITE)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer fid.Close()

	if _, err := fid.Write([]byte(req.Content)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// --- Message types ---

type Message struct {
	ID      string `json:"id"`
	From    string `json:"from"`
	To      string `json:"to"`
	Type    string `json:"type"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

type InboxMessage struct {
	Message
	Filename  string `json:"filename"`
	Timestamp int64  `json:"timestamp"`
}

// --- Inbox handlers ---

func handleInbox(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	fs, err := connect()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer fs.Close()

	messages := readMailbox(fs, "user/inbox")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(messages)
}

func handleInboxCount(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	fs, err := connect()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer fs.Close()

	fid, err := fs.Open("user/inbox", plan9.OREAD)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer fid.Close()

	count := 0
	for {
		dirs, err := fid.Dirread()
		if err != nil || len(dirs) == 0 {
			break
		}
		for _, d := range dirs {
			if strings.HasSuffix(d.Name, ".json") {
				count++
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"count": count})
}

func handleInboxReply(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		To      string `json:"to"`
		Type    string `json:"type"`
		Subject string `json:"subject"`
		Body    string `json:"body"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	fs, err := connect()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer fs.Close()

	msg := map[string]interface{}{
		"to":      req.To,
		"type":    req.Type,
		"subject": req.Subject,
		"body":    req.Body,
	}
	msgJSON, err := json.Marshal(msg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fid, err := fs.Open("user/mail", plan9.OWRITE)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer fid.Close()

	if _, err := fid.Write(msgJSON); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "sent"})
}

func handleInboxArchive(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Filename string `json:"filename"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	fs, err := connect()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer fs.Close()

	fid, err := fs.Open("user/ctl", plan9.OWRITE)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer fid.Close()

	if _, err := fid.Write([]byte(fmt.Sprintf("complete %s", req.Filename))); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "archived"})
}

func handleArchive(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	fs, err := connect()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer fs.Close()

	messages := readMailbox(fs, "user/completed")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(messages)
}

// readMailbox reads all .json messages from a 9P directory
func readMailbox(fs *client.Fsys, path string) []InboxMessage {
	fid, err := fs.Open(path, plan9.OREAD)
	if err != nil {
		return []InboxMessage{}
	}
	defer fid.Close()

	messages := make([]InboxMessage, 0)
	for {
		dirs, err := fid.Dirread()
		if err != nil || len(dirs) == 0 {
			break
		}
		for _, d := range dirs {
			if !strings.HasSuffix(d.Name, ".json") {
				continue
			}

			msgFid, err := fs.Open(filepath.Join(path, d.Name), plan9.OREAD)
			if err != nil {
				continue
			}
			data, err := io.ReadAll(msgFid)
			msgFid.Close()
			if err != nil {
				continue
			}

			var msg Message
			if err := json.Unmarshal(data, &msg); err != nil {
				continue
			}

			var ts int64
			parts := strings.Split(strings.TrimSuffix(d.Name, ".json"), "-")
			if len(parts) == 2 {
				fmt.Sscanf(parts[1], "%d", &ts)
			}

			messages = append(messages, InboxMessage{
				Message:   msg,
				Filename:  d.Name,
				Timestamp: ts,
			})
		}
	}
	return messages
}

// --- Beads handlers ---

func handleBeads(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	fs, err := connect()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer fs.Close()

	fid, err := fs.Open("beads/list", plan9.OREAD)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer fid.Close()

	data, err := io.ReadAll(fid)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func handleBeadsReady(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	fs, err := connect()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer fs.Close()

	fid, err := fs.Open("beads/ready", plan9.OREAD)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer fid.Close()

	data, err := io.ReadAll(fid)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func handleBeadsCtl(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Command string `json:"command"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	fs, err := connect()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer fs.Close()

	fid, err := fs.Open("beads/ctl", plan9.OWRITE)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer fid.Close()

	if _, err := fid.Write([]byte(req.Command)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleBeadDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/beads/")
	if id == "" {
		http.Error(w, "Invalid bead ID", http.StatusBadRequest)
		return
	}

	fs, err := connect()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer fs.Close()

	fid, err := fs.Open(filepath.Join("beads", id, "json"), plan9.OREAD)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer fid.Close()

	data, err := io.ReadAll(fid)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

// --- Daemon status handler ---

func handleDaemonStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	fs, err := connect()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"running": false,
			"error":   err.Error(),
		})
		return
	}
	defer fs.Close()

	fid, err := fs.Open("list", plan9.OREAD)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"running":       true,
			"session_count": 0,
		})
		return
	}
	defer fid.Close()

	data, _ := io.ReadAll(fid)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	count := 0
	for _, line := range lines {
		if line != "" {
			count++
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"running":       true,
		"session_count": count,
	})
}
