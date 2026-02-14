// anvilsrv - AnviLLM backend server daemon
package main

import (
	"anvillm/internal/backend"
	"anvillm/internal/backend/tmux"
	"anvillm/internal/backends"
	"anvillm/internal/p9"
	"anvillm/internal/session"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"9fans.net/go/plan9/client"
)

func getPidFilePath() string {
	ns := client.Namespace()
	if ns == "" {
		// Fallback to /tmp if namespace not available
		return filepath.Join("/tmp", "anvilsrv.pid")
	}
	return filepath.Join(ns, "anvilsrv.pid")
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "start":
		start(true) // daemonize
	case "fgstart":
		start(false) // foreground
	case "stop":
		stop()
	case "status":
		status()
	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: %s {start|fgstart|stop|status}\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "Commands:\n")
	fmt.Fprintf(os.Stderr, "  start   - Start the anvilsrv daemon (daemonized)\n")
	fmt.Fprintf(os.Stderr, "  fgstart - Start in foreground (for debugging)\n")
	fmt.Fprintf(os.Stderr, "  stop    - Stop the running daemon\n")
	fmt.Fprintf(os.Stderr, "  status  - Check daemon status\n")
}

func start(daemonize bool) {
	pidFile := getPidFilePath()

	// Check if already running before daemonizing
	if existingPid := readPidFile(); existingPid != 0 {
		if isProcessRunning(existingPid) {
			log.Fatalf("agent already running")
		}
		// Stale PID file
		os.Remove(pidFile)
	}

	// Daemonize if requested
	if daemonize {
		// Check if we're already the daemon (via environment variable)
		if os.Getenv("ANVILSRV_DAEMON") != "1" {
			// Fork the daemon process - use full path to executable
			cmd, err := os.Executable()
			if err != nil {
				log.Fatalf("Failed to get executable path: %v", err)
			}
			args := []string{"fgstart"} // Use fgstart to avoid re-daemonizing

			attr := &os.ProcAttr{
				Dir: "/",
				Env: append(os.Environ(), "ANVILSRV_DAEMON=1"),
				Files: []*os.File{nil, nil, nil}, // detach stdin/stdout/stderr
			}

			proc, err := os.StartProcess(cmd, append([]string{cmd}, args...), attr)
			if err != nil {
				log.Fatalf("Failed to daemonize: %v", err)
			}

			// Wait a moment to ensure daemon starts and writes PID file
			// The daemon will handle its own PID file
			proc.Release()

			// Give daemon time to start
			for i := 0; i < 10; i++ {
				if readPidFile() != 0 {
					fmt.Fprintf(os.Stderr, "anvilsrv started successfully\n")
					return
				}
				time.Sleep(100 * time.Millisecond)
			}
			fmt.Fprintf(os.Stderr, "anvilsrv started (daemon mode)\n")
			return
		}
	}

	// We're the daemon (or running in foreground), proceed with actual startup
	pid := os.Getpid()
	pidContent := fmt.Sprintf("%d\n", pid)

	// Try to create PID file exclusively (fails if exists)
	f, err := os.OpenFile(pidFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		// File exists, check if process is still running
		if existingPid := readPidFile(); existingPid != 0 {
			if isProcessRunning(existingPid) {
				log.Fatalf("anvilsrv already running (PID %d)", existingPid)
			}
			// Stale PID file, remove and retry
			os.Remove(pidFile)
			f, err = os.OpenFile(pidFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
			if err != nil {
				log.Fatalf("Failed to create PID file: %v", err)
			}
		} else {
			log.Fatalf("Failed to create PID file: %v", err)
		}
	}

	if _, err := f.WriteString(pidContent); err != nil {
		f.Close()
		os.Remove(pidFile)
		log.Fatalf("Failed to write PID file: %v", err)
	}
	f.Close()
	defer os.Remove(pidFile)

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Create all backends
	backendMap := map[string]backend.Backend{
		"kiro-cli": backends.NewKiroCLI(),
		"claude":   backends.NewClaude(),
	}

	mgr := session.NewManager(backendMap)

	// Cleanup tmux sessions on exit
	defer func() {
		log.Println("Shutting down: cleaning up tmux sessions")
		for _, b := range backendMap {
			if tmuxBackend, ok := b.(*tmux.Backend); ok {
				tmuxBackend.Cleanup()
			}
		}
	}()

	// Start 9P server
	srv, err := p9.NewServer(mgr)
	if err != nil {
		log.Fatal(err)
	}
	defer srv.Close()

	log.Println("anvilsrv started successfully")
	log.Printf("9P server listening on %s", srv.SocketPath())

	// Wait for termination signal
	sig := <-sigChan
	log.Printf("Received signal %v, shutting down", sig)
}

func stop() {
	pidFile := getPidFilePath()
	pid := readPidFile()
	if pid == 0 {
		fmt.Fprintf(os.Stderr, "anvilsrv is not running\n")
		os.Exit(1)
	}

	if !isProcessRunning(pid) {
		fmt.Fprintf(os.Stderr, "anvilsrv is not running (stale PID file)\n")
		os.Remove(pidFile)
		os.Exit(1)
	}

	// Send SIGTERM
	process, err := os.FindProcess(pid)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to find process: %v\n", err)
		os.Exit(1)
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to send SIGTERM: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Sent SIGTERM to anvilsrv (PID %d)\n", pid)
}

func status() {
	pid := readPidFile()
	if pid == 0 {
		fmt.Println("anvilsrv is not running")
		os.Exit(1)
	}

	if !isProcessRunning(pid) {
		fmt.Println("anvilsrv is not running (stale PID file)")
		os.Exit(1)
	}

	fmt.Printf("anvilsrv is running (PID %d)\n", pid)
}

// readPidFile reads the PID from the PID file, returns 0 if not found
func readPidFile() int {
	pidFile := getPidFilePath()
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	return pid
}

// isProcessRunning checks if a process with the given PID is running
func isProcessRunning(pid int) bool {
	// Send signal 0 to check if process exists
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil
}
