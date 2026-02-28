// anvilsrv - AnviLLM backend server daemon
package main

import (
	"anvillm/internal/backend"
	"anvillm/internal/backend/tmux"
	"anvillm/internal/backends"
	"anvillm/internal/logging"
	"anvillm/internal/p9"
	"anvillm/internal/session"
	"anvillm/internal/supervisor"
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"9fans.net/go/plan9/client"
	"go.uber.org/zap"
)

func getPidFilePath() string {
	ns := client.Namespace()
	if ns == "" {
		// Fallback to /tmp if namespace not available
		return filepath.Join("/tmp", "anvilsrv.pid")
	}
	return filepath.Join(ns, "anvilsrv.pid")
}

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
	// Initialize logging first
	if err := logging.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logging: %v\n", err)
		os.Exit(1)
	}
	defer logging.Logger().Sync()

	// Set CLAUDE_CONFIG_DIR if not already set in the environment
	if os.Getenv("CLAUDE_CONFIG_DIR") == "" {
		os.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(os.Getenv("HOME"), ".config", "anvillm", "claude"))
	}

	pidFile := getPidFilePath()

	// Check if already running before daemonizing
	if existingPid := readPidFile(); existingPid != 0 {
		if isProcessRunning(existingPid) {
			logging.Logger().Fatal("agent already running", zap.Int("pid", existingPid))
		}
		// Stale PID file
		logging.Logger().Warn("removing stale PID file", zap.Int("pid", existingPid))
		os.Remove(pidFile)
	}

	// Daemonize if requested
	if daemonize {
		// Check if we're already the daemon (via environment variable)
		if os.Getenv("ANVILSRV_DAEMON") != "1" {
			logging.Logger().Info("daemonizing anvilsrv")
			// Fork the daemon process - use full path to executable
			cmd, err := os.Executable()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to get executable path: %v\n", err)
				os.Exit(1)
			}
			args := []string{"fgstart"} // Use fgstart to avoid re-daemonizing

			attr := &os.ProcAttr{
				Dir: "/",
				Env: append(os.Environ(), "ANVILSRV_DAEMON=1"),
				Files: []*os.File{nil, nil, nil}, // detach stdin/stdout/stderr
			}

			proc, err := os.StartProcess(cmd, append([]string{cmd}, args...), attr)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to daemonize: %v\n", err)
				os.Exit(1)
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

	logging.Logger().Info("starting anvilsrv", zap.Int("pid", pid), zap.Bool("daemonized", daemonize))

	// Try to create PID file exclusively (fails if exists)
	f, err := os.OpenFile(pidFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		// File exists, check if process is still running
		if existingPid := readPidFile(); existingPid != 0 {
			if isProcessRunning(existingPid) {
				logging.Logger().Fatal("anvilsrv already running", zap.Int("pid", existingPid))
			}
			// Stale PID file, remove and retry
			logging.Logger().Warn("removing stale PID file on startup", zap.Int("pid", existingPid))
			os.Remove(pidFile)
			f, err = os.OpenFile(pidFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to create PID file: %v\n", err)
				os.Exit(1)
			}
		} else {
			fmt.Fprintf(os.Stderr, "Failed to create PID file: %v\n", err)
			os.Exit(1)
		}
	}

	if _, err := f.WriteString(pidContent); err != nil {
		f.Close()
		os.Remove(pidFile)
		logging.Logger().Fatal("failed to write PID file", zap.Error(err))
	}
	f.Close()
	defer os.Remove(pidFile)

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Create all backends
	nsSuffix := getNamespaceSuffix()
	logging.Logger().Debug("initializing backends", zap.String("namespace_suffix", nsSuffix))
	backendMap := map[string]backend.Backend{
		"kiro-cli": backends.NewKiroCLI(nsSuffix),
		"claude":   backends.NewClaude(nsSuffix),
		"ollama":   backends.NewOllama(nsSuffix),
	}

	mgr := session.NewManager(backendMap)
	logging.Logger().Info("session manager initialized")

	// Cleanup tmux sessions on exit
	defer func() {
		if r := recover(); r != nil {
			logging.Logger().Fatal("panic in main", zap.Any("panic", r))
		}
		logging.Logger().Info("shutting down: cleaning up tmux sessions")
		for _, b := range backendMap {
			if tmuxBackend, ok := b.(*tmux.Backend); ok {
				tmuxBackend.Cleanup()
			}
		}
	}()

	// Start 9P server with empty beads filesystem (mounts created on-demand)
	beadsFS := p9.NewBeadsFS(nil, context.Background())
	srv, err := p9.NewServer(mgr, beadsFS)
	if err != nil {
		logging.Logger().Fatal("failed to start 9P server", zap.Error(err))
	}
	defer srv.Close()

	// Wire up event bus to session manager
	mgr.SetEventBus(srv.Events())

	// Start supervisor
	ctx := context.Background()
	sup := supervisor.New(mgr, srv.Beads(), mgr.GetMailManager(), srv.Events())
	go sup.Run(ctx)

	logging.Logger().Info("anvilsrv started successfully", zap.String("socket", srv.SocketPath()))

	// Start background monitor for self-healing
	stopMonitor := make(chan struct{})
	logging.Logger().Debug("starting background session monitor")
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logging.Logger().Error("panic in monitor goroutine", zap.Any("panic", r))
			}
		}()
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				for _, id := range mgr.List() {
					if sess := mgr.Get(id); sess != nil {
						sess.Refresh(context.Background())
					}
				}
			case <-stopMonitor:
				logging.Logger().Debug("stopping background session monitor")
				return
			}
		}
	}()

	// Wait for termination signal
	sig := <-sigChan
	logging.Logger().Info("received signal, shutting down", zap.String("signal", sig.String()))
	close(stopMonitor)
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

	logging.Logger().Info("stopping anvilsrv", zap.Int("pid", pid))
	fmt.Printf("Stopping anvilsrv (PID %d)\n", pid)
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

	logging.Logger().Info("anvilsrv status check", zap.Int("pid", pid), zap.Bool("running", true))
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
