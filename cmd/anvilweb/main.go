package main

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"9fans.net/go/plan9"
	"9fans.net/go/plan9/client"
)

//go:embed static/*
var static embed.FS

var (
	addr      = flag.String("addr", ":8080", "HTTP server address")
	namespace = flag.String("ns", os.Getenv("NAMESPACE"), "9P namespace")
)

type Session struct {
	ID      string `json:"id"`
	Alias   string `json:"alias"`
	State   string `json:"state"`
	PID     string `json:"pid"`
	CWD     string `json:"cwd"`
	Backend string `json:"backend"`
}

func main() {
	flag.Parse()

	if *namespace == "" {
		*namespace = fmt.Sprintf("/tmp/ns.%s.:0", os.Getenv("USER"))
	}

	staticFS, err := fs.Sub(static, "static")
	if err != nil {
		log.Fatal(err)
	}
	http.Handle("/", http.FileServer(http.FS(staticFS)))
	http.HandleFunc("/api/sessions", handleSessions)
	http.HandleFunc("/api/session/", handleSession)
	http.HandleFunc("/api/log/", handleLogStream)

	log.Printf("Starting web server on %s", *addr)
	log.Printf("Using namespace: %s", *namespace)
	log.Fatal(http.ListenAndServe(*addr, nil))
}

func connect() (*client.Fsys, error) {
	return client.Mount("unix", filepath.Join(*namespace, "agent"))
}

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
		
		// Read backend for each session
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

	// Validate backend
	if req.Backend != "kiro-cli" && req.Backend != "claude" {
		http.Error(w, "Invalid backend", http.StatusBadRequest)
		return
	}

	// Validate path
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

	cmd := fmt.Sprintf("new %s %s", req.Backend, req.CWD)
	if _, err := fid.Write([]byte(cmd)); err != nil {
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
		if action == "prompt" {
			sendPrompt(w, r, id)
		} else if action == "ctl" {
			controlSession(w, r, id)
		} else {
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

	// Read state
	if fid, err := fs.Open(filepath.Join(id, "state"), plan9.OREAD); err == nil {
		if data, err := io.ReadAll(fid); err == nil {
			sess.State = strings.TrimSpace(string(data))
		}
		fid.Close()
	}

	// Read alias
	if fid, err := fs.Open(filepath.Join(id, "alias"), plan9.OREAD); err == nil {
		if data, err := io.ReadAll(fid); err == nil {
			sess.Alias = strings.TrimSpace(string(data))
		}
		fid.Close()
	}

	// Read backend
	if fid, err := fs.Open(filepath.Join(id, "backend"), plan9.OREAD); err == nil {
		if data, err := io.ReadAll(fid); err == nil {
			sess.Backend = strings.TrimSpace(string(data))
		}
		fid.Close()
	}

	// Read pid
	if fid, err := fs.Open(filepath.Join(id, "pid"), plan9.OREAD); err == nil {
		if data, err := io.ReadAll(fid); err == nil {
			sess.PID = strings.TrimSpace(string(data))
		}
		fid.Close()
	}

	// Read cwd
	if fid, err := fs.Open(filepath.Join(id, "cwd"), plan9.OREAD); err == nil {
		if data, err := io.ReadAll(fid); err == nil {
			sess.CWD = strings.TrimSpace(string(data))
		}
		fid.Close()
	}

	json.NewEncoder(w).Encode(sess)
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

	json.NewEncoder(w).Encode(map[string]string{
		"content": string(data),
	})
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

	// Validate command
	validCmds := map[string]bool{"stop": true, "restart": true, "kill": true, "refresh": true}
	if !validCmds[req.Command] {
		http.Error(w, "Invalid command", http.StatusBadRequest)
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


func handleLogStream(w http.ResponseWriter, r *http.Request) {
	// Extract session ID from path
	id := strings.TrimPrefix(r.URL.Path, "/api/log/")
	if id == "" {
		http.Error(w, "Invalid session ID", http.StatusBadRequest)
		return
	}

	// Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	fs, err := connect()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer fs.Close()

	// Open log file for streaming
	fid, err := fs.Open(filepath.Join(id, "log"), plan9.OREAD)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer fid.Close()

	// Flush immediately to establish connection
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	// Stream log data
	buf := make([]byte, 4096)
	for {
		n, err := fid.Read(buf)
		if n > 0 {
			// Escape data for SSE format - each line must be prefixed with "data: "
			data := string(buf[:n])
			lines := strings.Split(data, "\n")
			for _, line := range lines {
				if line != "" || len(lines) > 1 {
					fmt.Fprintf(w, "data: %s\n", line)
				}
			}
			fmt.Fprintf(w, "\n")
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		}
		if err != nil {
			// Connection closed or error
			break
		}
		
		// Check if client disconnected
		select {
		case <-r.Context().Done():
			return
		default:
		}
	}
}
