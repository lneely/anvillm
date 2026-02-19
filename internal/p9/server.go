// Package p9 implements the 9P filesystem for Q agent sessions.
package p9

import (
	"anvillm/internal/backend"
	"anvillm/internal/backend/tmux"
	"anvillm/internal/events"
	"anvillm/internal/mailbox"
	"anvillm/internal/session"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/google/uuid"

	"9fans.net/go/plan9"
	"9fans.net/go/plan9/client"
)

/*
Filesystem layout:

agent/
    ctl                 (write) "new <backend> <cwd>" creates session, returns id
    list                (read)  list sessions: "id alias state pid cwd"
    user/               (dir)   special user mailbox (singleton)
        inbox/          (dir)   messages FROM bots TO user
        outbox/         (dir)   messages FROM user TO bots
        completed/      (dir)   processed messages
    {session-id}/
        ctl             (write) "stop", "restart", "kill", "refresh"
        in              (write) send prompt (non-blocking, validates and returns immediately)
        out             (write) bot writes response summary (includes actual response + tool usage summary)
        log             (read)  streaming chat history (USER:/ASSISTANT: with --- separators, blocks like tail -f)
        state           (read)  "starting", "idle", "running", "stopped", "error", "exited"
        pid             (read)  process id
        cwd             (read)  working directory
        alias           (r/w)   session alias
        backend         (read)  backend name (e.g., "kiro-cli", "claude")
        context         (r/w)   text prepended to every prompt

Communication:
    All communication goes through mailboxes (outbox -> inbox).
    "user" is a special participant (not a session).
    bot -> user: bot writes to its outbox with to="user"
    user -> bot: user writes to user/outbox with to="{session-id}"
    When processing user inbox, message body is written to sender's log file.
*/

const (
	QTDir  = plan9.QTDIR
	QTFile = plan9.QTFILE
)

// Qid paths
const (
	qidRoot = iota
	qidCtl
	qidList
	qidStatus
	qidAudit                     // agent/audit
	qidEvents                    // agent/events
	qidUser                      // user directory
	qidUserInbox                 // user/inbox
	qidUserOutbox                // user/outbox
	qidUserCompleted             // user/completed
	qidUserCtl                   // user/ctl
	qidUserMail                  // user/mail
	qidSessionBase   = 1000
	qidPeersBase     = 0x10000000 // peers/{id}/file
	qidInboxBase     = 0x20000000 // session/{id}/inbox
	qidOutboxBase    = 0x30000000 // session/{id}/outbox
	qidCompletedBase = 0x40000000 // session/{id}/completed
	qidMessageBase   = 0x50000000 // message files
)

// File indices within a session directory
const (
	fileCtl = iota
	fileState
	filePid
	fileCwd
	fileAlias
	fileBackend
	fileContext
	fileRole
	fileTasks
	fileTmux
	fileMail
	fileCount
)

var fileNames = []string{"ctl", "state", "pid", "cwd", "alias", "backend", "context", "role", "tasks", "tmux", "mail"}

// Directory names in session
var dirNames = []string{"inbox", "outbox", "completed"}

type Server struct {
	mgr           *session.Manager
	listener      net.Listener
	socketPath    string
	events        *events.Queue
	OnAliasChange func(backend.Session) // Called when session alias changes
	mu            sync.RWMutex
}

type connState struct {
	fids      map[uint32]*fid
	mu        sync.RWMutex
	sessionID string // Track which session owns this connection
}

type fid struct {
	qid    plan9.Qid
	path   string
	mode   uint8
	offset int64
}

// NewServer creates and starts the 9P server.
func NewServer(mgr *session.Manager) (*Server, error) {
	ns := client.Namespace()
	if ns == "" {
		return nil, fmt.Errorf("no namespace")
	}

	sockPath := filepath.Join(ns, "agent")

	// Remove stale socket
	if _, err := os.Stat(sockPath); err == nil {
		conn, err := net.Dial("unix", sockPath)
		if err == nil {
			conn.Close()
			return nil, fmt.Errorf("agent already running")
		}
		os.Remove(sockPath)
	}

	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		return nil, err
	}

	s := &Server{mgr: mgr, listener: listener, socketPath: sockPath, events: events.NewQueue()}
	go s.acceptLoop()
	return s, nil
}

// SocketPath returns the path to the Unix socket
func (s *Server) SocketPath() string {
	return s.socketPath
}

// Events returns the event queue for pushing events.
func (s *Server) Events() *events.Queue {
	return s.events
}

func (s *Server) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				fmt.Fprintf(os.Stderr, "accept: %v\n", err)
			}
			return
		}
		go s.serve(conn)
	}
}

func (s *Server) serve(conn net.Conn) {
	defer conn.Close()
	cs := &connState{fids: make(map[uint32]*fid)}

	for {
		fc, err := plan9.ReadFcall(conn)
		if err != nil {
			if err != io.EOF {
				fmt.Fprintf(os.Stderr, "read: %v\n", err)
			}
			return
		}

		rfc := s.handle(cs, fc)
		if err := plan9.WriteFcall(conn, rfc); err != nil {
			fmt.Fprintf(os.Stderr, "write: %v\n", err)
			return
		}
	}
}

func (s *Server) handle(cs *connState, fc *plan9.Fcall) *plan9.Fcall {
	switch fc.Type {
	case plan9.Tversion:
		return &plan9.Fcall{Type: plan9.Rversion, Tag: fc.Tag, Msize: fc.Msize, Version: "9P2000"}
	case plan9.Tauth:
		return errFcall(fc, "no auth required")
	case plan9.Tattach:
		return s.attach(cs, fc)
	case plan9.Twalk:
		return s.walk(cs, fc)
	case plan9.Topen:
		return s.open(cs, fc)
	case plan9.Tcreate:
		return s.create(cs, fc)
	case plan9.Tread:
		return s.read(cs, fc)
	case plan9.Twrite:
		return s.write(cs, fc)
	case plan9.Tremove:
		return s.remove(cs, fc)
	case plan9.Tstat:
		return s.stat(cs, fc)
	case plan9.Tclunk:
		cs.mu.Lock()
		delete(cs.fids, fc.Fid)
		cs.mu.Unlock()
		return &plan9.Fcall{Type: plan9.Rclunk, Tag: fc.Tag}
	default:
		return errFcall(fc, "not supported")
	}
}

func (s *Server) attach(cs *connState, fc *plan9.Fcall) *plan9.Fcall {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	qid := plan9.Qid{Type: QTDir, Path: qidRoot}
	cs.fids[fc.Fid] = &fid{qid: qid, path: "/"}
	return &plan9.Fcall{Type: plan9.Rattach, Tag: fc.Tag, Qid: qid}
}

func (s *Server) walk(cs *connState, fc *plan9.Fcall) *plan9.Fcall {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	f, ok := cs.fids[fc.Fid]
	if !ok {
		return errFcall(fc, "bad fid")
	}

	if len(fc.Wname) == 0 {
		cs.fids[fc.Newfid] = &fid{qid: f.qid, path: f.path}
		return &plan9.Fcall{Type: plan9.Rwalk, Tag: fc.Tag, Wqid: []plan9.Qid{}}
	}

	path := f.path
	var qids []plan9.Qid

	for _, name := range fc.Wname {
		var qid plan9.Qid
		var newPath string

		if path == "/" {
			switch name {
			case "ctl":
				qid = plan9.Qid{Type: QTFile, Path: qidCtl}
				newPath = "/ctl"
			case "list":
				qid = plan9.Qid{Type: QTFile, Path: qidList}
				newPath = "/list"
			case "audit":
				qid = plan9.Qid{Type: QTFile, Path: qidAudit}
				newPath = "/audit"
			case "events":
				qid = plan9.Qid{Type: QTFile, Path: qidEvents}
				newPath = "/events"
			case "user":
				qid = plan9.Qid{Type: QTDir, Path: qidUser}
				newPath = "/user"
			default:
				// Check if it's a session ID
				if sess := s.mgr.Get(name); sess != nil {
					qid = plan9.Qid{Type: QTDir, Path: qidSessionBase + hashID(name)}
					newPath = "/" + name
				} else {
					return errFcall(fc, "not found")
				}
			}
		} else if path == "/user" {
			// Inside user directory - only mailbox subdirs
			switch name {
			case "inbox":
				qid = plan9.Qid{Type: QTDir, Path: qidUserInbox}
				newPath = "/user/inbox"
			case "outbox":
				qid = plan9.Qid{Type: QTDir, Path: qidUserOutbox}
				newPath = "/user/outbox"
			case "completed":
				qid = plan9.Qid{Type: QTDir, Path: qidUserCompleted}
				newPath = "/user/completed"
			case "ctl":
				qid = plan9.Qid{Type: QTFile, Path: qidUserCtl}
				newPath = "/user/ctl"
			case "mail":
				qid = plan9.Qid{Type: QTFile, Path: qidUserMail}
				newPath = "/user/mail"
			default:
				return errFcall(fc, "not found")
			}
		} else if strings.HasPrefix(path, "/user/") && strings.Count(path, "/") == 2 {
			// Inside user mailbox directory - message files
			qid = plan9.Qid{Type: QTFile, Path: qidMessageBase + hashID("user"+path[6:]+name)}
			newPath = path + "/" + name
		} else if strings.Count(path, "/") == 1 && path != "/" {
			// Inside a session directory
			sessID := strings.TrimPrefix(path, "/")
			if s.mgr.Get(sessID) == nil {
				return errFcall(fc, "session not found")
			}
			
			// Check if it's a mailbox directory
			if name == "inbox" {
				qid = plan9.Qid{Type: QTDir, Path: qidInboxBase + hashID(sessID)}
				newPath = path + "/inbox"
			} else if name == "outbox" {
				qid = plan9.Qid{Type: QTDir, Path: qidOutboxBase + hashID(sessID)}
				newPath = path + "/outbox"
			} else if name == "completed" {
				qid = plan9.Qid{Type: QTDir, Path: qidCompletedBase + hashID(sessID)}
				newPath = path + "/completed"
			} else {
				// Regular session file
				idx := fileIndex(name)
				if idx < 0 {
					return errFcall(fc, "not found")
				}
				qid = plan9.Qid{Type: QTFile, Path: qidSessionBase + hashID(sessID)*fileCount + uint64(idx)}
				newPath = path + "/" + name
			}
		} else if strings.Count(path, "/") == 2 {
			// Inside a mailbox directory (inbox/outbox/completed)
			parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
			sessID := parts[0]
			mailbox := parts[1]
			
			if s.mgr.Get(sessID) == nil {
				return errFcall(fc, "session not found")
			}
			
			// Message file (msg-*.json)
			qid = plan9.Qid{Type: QTFile, Path: qidMessageBase + hashID(sessID+mailbox+name)}
			newPath = path + "/" + name
		} else {
			return errFcall(fc, "not found")
		}

		qids = append(qids, qid)
		path = newPath
	}

	cs.fids[fc.Newfid] = &fid{qid: qids[len(qids)-1], path: path}
	return &plan9.Fcall{Type: plan9.Rwalk, Tag: fc.Tag, Wqid: qids}
}

func (s *Server) open(cs *connState, fc *plan9.Fcall) *plan9.Fcall {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	f, ok := cs.fids[fc.Fid]
	if !ok {
		return errFcall(fc, "bad fid")
	}
	f.mode = fc.Mode
	f.offset = 0
	return &plan9.Fcall{Type: plan9.Ropen, Tag: fc.Tag, Qid: f.qid}
}

func (s *Server) create(cs *connState, fc *plan9.Fcall) *plan9.Fcall {
	return errFcall(fc, "create not supported")
}

func (s *Server) read(cs *connState, fc *plan9.Fcall) *plan9.Fcall {
	cs.mu.Lock()
	f, ok := cs.fids[fc.Fid]
	cs.mu.Unlock()
	if !ok {
		return errFcall(fc, "bad fid")
	}

	var data []byte

	if f.qid.Type&QTDir != 0 {
		data = s.readDir(f.path, fc.Offset, fc.Count)
	} else if f.path == "/audit" {
		// Streaming audit log (like tail -f)
		data = s.readAuditLog(fc.Offset, fc.Count)
	} else if f.path == "/events" {
		// Event queue - return all pending events
		data = s.events.Read()
	} else {
		content := s.readFile(f.path)
		if fc.Offset < uint64(len(content)) {
			end := min(int(fc.Offset)+int(fc.Count), len(content))
			data = []byte(content[fc.Offset:end])
		}
	}

	return &plan9.Fcall{Type: plan9.Rread, Tag: fc.Tag, Count: uint32(len(data)), Data: data}
}

func (s *Server) write(cs *connState, fc *plan9.Fcall) *plan9.Fcall {
	cs.mu.Lock()
	f, ok := cs.fids[fc.Fid]
	cs.mu.Unlock()
	if !ok {
		return errFcall(fc, "bad fid")
	}

	input := strings.TrimSpace(string(fc.Data))
	parts := strings.Split(strings.TrimPrefix(f.path, "/"), "/")
	
	fmt.Fprintf(os.Stderr, "[DEBUG] write: path=%q parts=%v len=%d\n", f.path, parts, len(parts))

	// /events - ack events
	if f.path == "/events" {
		if err := s.events.Ack(fc.Data); err != nil {
			return errFcall(fc, err.Error())
		}
		return &plan9.Fcall{Type: plan9.Rwrite, Tag: fc.Tag, Count: uint32(len(fc.Data))}
	}

	// /ctl - create new session
	if f.path == "/ctl" {
		args := strings.Fields(input)
		fmt.Fprintf(os.Stderr, "[DEBUG] ctl write: input=%q args=%v len=%d fc.Count=%d len(fc.Data)=%d\n",
			input, args, len(args), fc.Count, len(fc.Data))
		if len(args) < 2 || args[0] != "new" {
			return errFcall(fc, "usage: new <backend> <cwd> [role=<role>] [tasks=<task1,task2>]")
		}
		
		backendName := args[1]
		cwd, _ := os.Getwd()
		var role string
		var tasks []string
		
		// Parse remaining arguments: first non-key=value is cwd, rest are options
		cwdSet := false
		for i := 2; i < len(args); i++ {
			arg := args[i]
			if strings.HasPrefix(arg, "role=") {
				role = strings.TrimPrefix(arg, "role=")
			} else if strings.HasPrefix(arg, "tasks=") {
				taskStr := strings.TrimPrefix(arg, "tasks=")
				if taskStr != "" {
					tasks = strings.Split(taskStr, ",")
				}
			} else if !cwdSet {
				// First positional argument is cwd
				cwd = strings.Trim(arg, `"`)
				cwdSet = true
			} else {
				return errFcall(fc, fmt.Sprintf("unexpected argument: %s", arg))
			}
		}

		// Validate and clean the path
		cleanPath := filepath.Clean(cwd)

		// Ensure it's an absolute path
		if !filepath.IsAbs(cleanPath) {
			var err error
			cleanPath, err = filepath.Abs(cleanPath)
			if err != nil {
				return errFcall(fc, fmt.Sprintf("invalid path: %v", err))
			}
		}

		// Verify the directory exists
		if info, err := os.Stat(cleanPath); err != nil {
			return errFcall(fc, fmt.Sprintf("path does not exist: %v", err))
		} else if !info.IsDir() {
			return errFcall(fc, fmt.Sprintf("path is not a directory: %s", cleanPath))
		}

		opts := backend.SessionOptions{
			CWD:   cleanPath,
			Role:  role,
			Tasks: tasks,
		}
		_, err := s.mgr.New(opts, backendName)
		if err != nil {
			return errFcall(fc, err.Error())
		}
		fmt.Fprintf(os.Stderr, "[DEBUG] ctl write: session created, returning Count=%d\n", uint32(len(fc.Data)))
		return &plan9.Fcall{Type: plan9.Rwrite, Tag: fc.Tag, Count: uint32(len(fc.Data))}
	}

	// /{id}/ctl - session control
	if len(parts) == 2 && parts[1] == "ctl" {
		// Handle user/ctl specially
		if parts[0] == "user" {
			args := strings.Fields(input)
			if len(args) == 0 {
				return errFcall(fc, "usage: complete <msg-id>")
			}
			switch args[0] {
			case "complete":
				if len(args) < 2 {
					return errFcall(fc, "usage: complete <msg-id>")
				}
				msgID := args[1]
				mailMgr := s.mgr.GetMailManager()
				if err := mailMgr.CompleteMessage("user", msgID); err != nil {
					return errFcall(fc, err.Error())
				}
			default:
				return errFcall(fc, "unknown command")
			}
			return &plan9.Fcall{Type: plan9.Rwrite, Tag: fc.Tag, Count: uint32(len(fc.Data))}
		}
		
		sess := s.mgr.Get(parts[0])
		if sess == nil {
			return errFcall(fc, "session not found")
		}
		args := strings.Fields(input)
		if len(args) == 0 {
			return errFcall(fc, "usage: stop | restart | kill | refresh | complete <msg-id>")
		}
		switch args[0] {
		case "stop":
			ctx := context.Background()
			if err := sess.Stop(ctx); err != nil {
				return errFcall(fc, err.Error())
			}
		case "restart":
			ctx := context.Background()
			if err := sess.Restart(ctx); err != nil {
				return errFcall(fc, err.Error())
			}
		case "kill":
			sess.Close()
			s.mgr.Remove(sess.ID())
		case "refresh":
			ctx := context.Background()
			if err := sess.Refresh(ctx); err != nil {
				return errFcall(fc, err.Error())
			}
		case "complete":
			if len(args) < 2 {
				return errFcall(fc, "usage: complete <msg-id>")
			}
			msgID := args[1]
			mailMgr := s.mgr.GetMailManager()
			if err := mailMgr.CompleteMessage(parts[0], msgID); err != nil {
				return errFcall(fc, err.Error())
			}
		default:
			return errFcall(fc, "unknown command")
		}
		return &plan9.Fcall{Type: plan9.Rwrite, Tag: fc.Tag, Count: uint32(len(fc.Data))}
	}

	// /{id}/alias - set session alias
	if len(parts) == 2 && parts[1] == "alias" {
		sess := s.mgr.Get(parts[0])
		if sess == nil {
			return errFcall(fc, "session not found")
		}
		// Validate alias: alphanumeric, hyphen, underscore only
		matched, _ := regexp.MatchString(`^[A-Za-z0-9_-]+$`, input)
		if !matched {
			return errFcall(fc, "invalid alias: must match [A-Za-z0-9_-]+")
		}
		sess.SetAlias(input)
		if s.OnAliasChange != nil {
			s.OnAliasChange(sess)
		}
		return &plan9.Fcall{Type: plan9.Rwrite, Tag: fc.Tag, Count: uint32(len(fc.Data))}
	}

	// /{id}/context - set context prefix
	if len(parts) == 2 && parts[1] == "context" {
		sess := s.mgr.Get(parts[0])
		if sess == nil {
			return errFcall(fc, "session not found")
		}
		if tmuxSess, ok := sess.(*tmux.Session); ok {
			tmuxSess.SetContext(input)
		}
		return &plan9.Fcall{Type: plan9.Rwrite, Tag: fc.Tag, Count: uint32(len(fc.Data))}
	}

	// /{id}/state - set session state (with validation)
	if len(parts) == 2 && parts[1] == "state" {
		sessID := parts[0]
		sess := s.mgr.Get(sessID)
		if sess == nil {
			return errFcall(fc, "session not found")
		}

		// Validate state: only allow valid state values
		validStates := map[string]bool{
			"idle":     true,
			"running":  true,
			"stopped":  true,
			"starting": true,
			"error":    true,
			"exited":   true,
		}

		if !validStates[input] {
			return errFcall(fc, fmt.Sprintf("invalid state: %q (must be one of: idle, running, stopped, starting, error, exited)", input))
		}

		if tmuxSess, ok := sess.(*tmux.Session); ok {
			if err := tmuxSess.TransitionTo(input); err != nil {
				return errFcall(fc, fmt.Sprintf("invalid state transition: %v", err))
			}
		}
		
		// Track this connection's session ID
		cs.mu.Lock()
		if cs.sessionID == "" {
			cs.sessionID = sessID
		}
		cs.mu.Unlock()
		
		return &plan9.Fcall{Type: plan9.Rwrite, Tag: fc.Tag, Count: uint32(len(fc.Data))}
	}

	// /{id}/mail - write message to outbox (generates UUID filename)
	if len(parts) == 2 && parts[1] == "mail" {
		sessID := parts[0]
		
		// Track this connection's session ID
		cs.mu.Lock()
		if cs.sessionID == "" {
			cs.sessionID = sessID
		}
		cs.mu.Unlock()
		
		// Parse JSON message
		msg, err := mailbox.FromJSON(fc.Data)
		if err != nil {
			return errFcall(fc, fmt.Sprintf("invalid message JSON: %v", err))
		}
		
		// Validate message type
		if err := mailbox.ValidateMessageType(msg.Type); err != nil {
			return errFcall(fc, err.Error())
		}
		
		// Generate UUID for message
		msg.ID = uuid.New().String()
		
		// Add to outbox
		mailMgr := s.mgr.GetMailManager()
		if mailMgr == nil {
			return errFcall(fc, "mailbox not available")
		}
		
		if err := mailMgr.AddToOutbox(sessID, msg); err != nil {
			return errFcall(fc, fmt.Sprintf("failed to add message: %v", err))
		}
		
		// Transition sender to idle after sending any outbox message
		if sess := s.mgr.Get(sessID); sess != nil {
			if tmuxSess, ok := sess.(*tmux.Session); ok {
				tmuxSess.TransitionTo("idle")
			}
		}
		
		return &plan9.Fcall{Type: plan9.Rwrite, Tag: fc.Tag, Count: uint32(len(fc.Data))}
	}

	// /user/mail - write message from user to bot (generates UUID filename)
	if len(parts) == 2 && parts[0] == "user" && parts[1] == "mail" {
		// Parse JSON message
		msg, err := mailbox.FromJSON(fc.Data)
		if err != nil {
			return errFcall(fc, fmt.Sprintf("invalid message JSON: %v", err))
		}
		
		// Validate message type
		if err := mailbox.ValidateMessageType(msg.Type); err != nil {
			return errFcall(fc, err.Error())
		}
		
		// Set from field to "user"
		msg.From = "user"
		
		// Generate UUID for message
		msg.ID = uuid.New().String()
		
		// Add to user's outbox
		mailMgr := s.mgr.GetMailManager()
		if mailMgr == nil {
			return errFcall(fc, "mailbox not available")
		}
		
		if err := mailMgr.AddToOutbox("user", msg); err != nil {
			return errFcall(fc, fmt.Sprintf("failed to add message: %v", err))
		}
		
		return &plan9.Fcall{Type: plan9.Rwrite, Tag: fc.Tag, Count: uint32(len(fc.Data))}
	}

	// /agent/{id}/mail - write message from agent (generates UUID filename)
	if len(parts) == 2 && parts[1] == "mail" {
		sessID := parts[0]
		if s.mgr.Get(sessID) == nil {
			return errFcall(fc, "session not found")
		}
		
		// Parse JSON message
		msg, err := mailbox.FromJSON(fc.Data)
		if err != nil {
			return errFcall(fc, fmt.Sprintf("invalid message JSON: %v", err))
		}
		
		// Validate message type
		if err := mailbox.ValidateMessageType(msg.Type); err != nil {
			return errFcall(fc, err.Error())
		}
		
		// Set from field to session ID
		msg.From = sessID
		
		// Generate UUID for message
		msg.ID = uuid.New().String()
		
		// Add to agent's outbox
		mailMgr := s.mgr.GetMailManager()
		if mailMgr == nil {
			return errFcall(fc, "mailbox not available")
		}
		
		if err := mailMgr.AddToOutbox(sessID, msg); err != nil {
			return errFcall(fc, fmt.Sprintf("failed to add message: %v", err))
		}
		
		return &plan9.Fcall{Type: plan9.Rwrite, Tag: fc.Tag, Count: uint32(len(fc.Data))}
	}

	return errFcall(fc, "read-only")
}

func (s *Server) stat(cs *connState, fc *plan9.Fcall) *plan9.Fcall {
	cs.mu.RLock()
	f, ok := cs.fids[fc.Fid]
	cs.mu.RUnlock()
	if !ok {
		return errFcall(fc, "bad fid")
	}

	dir := s.pathToDir(f.path, f.qid)
	stat, _ := dir.Bytes()
	return &plan9.Fcall{Type: plan9.Rstat, Tag: fc.Tag, Stat: stat}
}

func (s *Server) remove(cs *connState, fc *plan9.Fcall) *plan9.Fcall {
	cs.mu.Lock()
	f, ok := cs.fids[fc.Fid]
	sessionID := cs.sessionID
	cs.mu.Unlock()
	if !ok {
		return errFcall(fc, "bad fid")
	}
	
	// Only support removing inbox messages (marks them as completed)
	parts := strings.Split(strings.TrimPrefix(f.path, "/"), "/")
	if len(parts) == 3 && parts[1] == "inbox" && strings.HasSuffix(parts[2], ".json") {
		sessID := parts[0]
		msgID := strings.TrimSuffix(parts[2], ".json")
		
		// Validate ownership: only allow removing from own inbox or user inbox
		if sessionID == "" || (sessID != "user" && sessID != sessionID) {
			return errFcall(fc, "permission denied: can only remove from own inbox")
		}
		
		mailMgr := s.mgr.GetMailManager()
		if mailMgr == nil {
			return errFcall(fc, "mailbox not available")
		}
		
		if err := mailMgr.CompleteMessage(sessID, msgID); err != nil {
			return errFcall(fc, err.Error())
		}
		
		cs.mu.Lock()
		delete(cs.fids, fc.Fid)
		cs.mu.Unlock()
		
		return &plan9.Fcall{Type: plan9.Rremove, Tag: fc.Tag}
	}
	
	return errFcall(fc, "remove not supported for this file")
}

func (s *Server) readDir(path string, offset uint64, count uint32) []byte {
	var dirs []plan9.Dir

	if path == "/" {
		dirs = append(dirs, plan9.Dir{
			Qid: plan9.Qid{Type: QTFile, Path: qidCtl}, Mode: 0222, Name: "ctl",
			Uid: "q", Gid: "q", Muid: "q",
		})
		dirs = append(dirs, plan9.Dir{
			Qid: plan9.Qid{Type: QTFile, Path: qidList}, Mode: 0444, Name: "list",
			Uid: "q", Gid: "q", Muid: "q",
		})
		dirs = append(dirs, plan9.Dir{
			Qid: plan9.Qid{Type: QTFile, Path: qidStatus}, Mode: 0444, Name: "status",
			Uid: "q", Gid: "q", Muid: "q",
		})
		dirs = append(dirs, plan9.Dir{
			Qid: plan9.Qid{Type: QTFile, Path: qidAudit}, Mode: 0444, Name: "audit",
			Uid: "q", Gid: "q", Muid: "q",
		})
		dirs = append(dirs, plan9.Dir{
			Qid: plan9.Qid{Type: QTFile, Path: qidEvents}, Mode: 0644, Name: "events",
			Uid: "q", Gid: "q", Muid: "q",
		})
		dirs = append(dirs, plan9.Dir{
			Qid:  plan9.Qid{Type: QTDir, Path: qidUser},
			Mode: plan9.DMDIR | 0555, Name: "user", Uid: "q", Gid: "q", Muid: "q",
		})
		for _, id := range s.mgr.List() {
			dirs = append(dirs, plan9.Dir{
				Qid:  plan9.Qid{Type: QTDir, Path: qidSessionBase + hashID(id)},
				Mode: plan9.DMDIR | 0555, Name: id, Uid: "q", Gid: "q", Muid: "q",
			})
		}
	} else if path == "/user" {
		// User directory - only mailbox subdirs
		dirs = append(dirs, plan9.Dir{
			Qid:  plan9.Qid{Type: QTDir, Path: qidUserInbox},
			Mode: plan9.DMDIR | 0555, Name: "inbox", Uid: "q", Gid: "q", Muid: "q",
		})
		dirs = append(dirs, plan9.Dir{
			Qid:  plan9.Qid{Type: QTDir, Path: qidUserOutbox},
			Mode: plan9.DMDIR | 0555, Name: "outbox", Uid: "q", Gid: "q", Muid: "q",
		})
		dirs = append(dirs, plan9.Dir{
			Qid:  plan9.Qid{Type: QTDir, Path: qidUserCompleted},
			Mode: plan9.DMDIR | 0555, Name: "completed", Uid: "q", Gid: "q", Muid: "q",
		})
		dirs = append(dirs, plan9.Dir{
			Qid:  plan9.Qid{Type: QTFile, Path: qidUserCtl},
			Mode: 0222, Name: "ctl", Uid: "q", Gid: "q", Muid: "q",
		})
		dirs = append(dirs, plan9.Dir{
			Qid:  plan9.Qid{Type: QTFile, Path: qidUserMail},
			Mode: 0222, Name: "mail", Uid: "q", Gid: "q", Muid: "q",
		})
	} else if strings.HasPrefix(path, "/user/") && strings.Count(path, "/") == 2 {
		// User mailbox directory (inbox/outbox/completed)
		mailboxType := strings.TrimPrefix(path, "/user/")
		
		mailMgr := s.mgr.GetMailManager()
		if mailMgr != nil {
			var messages []*mailbox.Message
			if mailboxType == "inbox" {
				messages = mailMgr.GetInbox("user")
			} else if mailboxType == "outbox" {
				messages = mailMgr.GetOutbox("user")
			} else if mailboxType == "completed" {
				messages = mailMgr.GetCompleted("user")
			}
			
			// Sort messages by ID (which is now timestamp-based)
			sort.Slice(messages, func(i, j int) bool {
				return messages[i].ID < messages[j].ID
			})
			
			for _, msg := range messages {
				data, _ := msg.ToJSON()
				dirs = append(dirs, plan9.Dir{
					Qid:    plan9.Qid{Type: QTFile, Path: qidMessageBase + hashID("user"+mailboxType+msg.ID)},
					Mode:   0644,
					Name:   msg.ID + ".json",
					Length: uint64(len(data)),
					Uid:    "q", Gid: "q", Muid: "q",
				})
			}
		}
	} else if strings.Count(path, "/") == 1 {
		// Session directory
		sessID := strings.TrimPrefix(path, "/")
		sess := s.mgr.Get(sessID)
		if sess == nil {
			return nil
		}
		// Add regular files
		for i, name := range fileNames {
			mode := uint32(0444)
			if name == "ctl" || name == "in" || name == "out" {
				mode = 0222
			} else if name == "alias" || name == "context" || name == "state" {
				mode = 0644
			}
			content := s.getSessionFile(sess, i)
			dirs = append(dirs, plan9.Dir{
				Qid:    plan9.Qid{Type: QTFile, Path: qidSessionBase + hashID(sessID)*fileCount + uint64(i)},
				Mode:   plan9.Perm(mode),
				Name:   name,
				Length: uint64(len(content)),
				Uid:    "q", Gid: "q", Muid: "q",
			})
		}
		// Add mailbox directories
		for _, dirName := range dirNames {
			var qidBase uint64
			var mode uint32
			if dirName == "inbox" {
				qidBase = qidInboxBase
				mode = 0555 // read-only (can list and read files)
			} else if dirName == "outbox" {
				qidBase = qidOutboxBase
				mode = 0555 // read-only (can list and read files)
			} else {
				qidBase = qidCompletedBase
				mode = 0555 // read-only (can list and read files)
			}
			dirs = append(dirs, plan9.Dir{
				Qid:  plan9.Qid{Type: QTDir, Path: qidBase + hashID(sessID)},
				Mode: plan9.DMDIR | plan9.Perm(mode), Name: dirName,
				Uid:  "q", Gid: "q", Muid: "q",
			})
		}
	} else if strings.Count(path, "/") == 2 {
		// Mailbox directory (inbox/outbox/completed)
		parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
		sessID := parts[0]
		mailboxType := parts[1]
		
		mailMgr := s.mgr.GetMailManager()
		if mailMgr == nil {
			return nil
		}
		
		var messages []*mailbox.Message
		if mailboxType == "inbox" {
			messages = mailMgr.GetInbox(sessID)
		} else if mailboxType == "outbox" {
			messages = mailMgr.GetOutbox(sessID)
		} else if mailboxType == "completed" {
			messages = mailMgr.GetCompleted(sessID)
		}
		
		// Sort messages by ID (which is now timestamp-based)
		sort.Slice(messages, func(i, j int) bool {
			return messages[i].ID < messages[j].ID
		})
		
		for _, msg := range messages {
			data, _ := msg.ToJSON()
			dirs = append(dirs, plan9.Dir{
				Qid:    plan9.Qid{Type: QTFile, Path: qidMessageBase + hashID(sessID+mailboxType+msg.ID)},
				Mode:   0644,
				Name:   msg.ID + ".json",
				Length: uint64(len(data)),
				Uid:    "q", Gid: "q", Muid: "q",
			})
		}
	}

	var data []byte
	for _, d := range dirs {
		b, _ := d.Bytes()
		data = append(data, b...)
	}
	if offset >= uint64(len(data)) {
		return nil
	}
	end := min(int(offset)+int(count), len(data))
	return data[offset:end]
}

func (s *Server) readAuditLog(offset uint64, count uint32) []byte {
	mailMgr := s.mgr.GetMailManager()
	if mailMgr == nil {
		return nil
	}

	auditLog := mailMgr.GetAuditLog()
	if auditLog == nil {
		return nil
	}

	// Try to read from current offset
	logData, hasMore := auditLog.ReadFrom(int64(offset))
	if hasMore && len(logData) > 0 {
		// Return available data
		end := min(int(count), len(logData))
		return []byte(logData[:end])
	}

	// No data available at this offset, wait for new data
	waitCh := auditLog.WaitForData()
	<-waitCh

	// Check again after new data arrived
	logData, hasMore = auditLog.ReadFrom(int64(offset))
	if hasMore && len(logData) > 0 {
		end := min(int(count), len(logData))
		return []byte(logData[:end])
	}

	return nil
}

func (s *Server) readFile(path string) string {
	if path == "/list" {
		var lines []string
		for _, id := range s.mgr.List() {
			sess := s.mgr.Get(id)
			if sess != nil {
				meta := sess.Metadata()
				alias := meta.Alias
				if alias == "" {
					alias = "-"
				}
				backend := meta.Backend
				if backend == "" {
					backend = "-"
				}
				lines = append(lines, fmt.Sprintf("%s\t%s\t%s\t%s\t%s", sess.ID(), backend, sess.State(), alias, meta.Cwd))
			}
		}
		return strings.Join(lines, "\n") + "\n"
	}

	if path == "/audit" {
		mailMgr := s.mgr.GetMailManager()
		if mailMgr == nil {
			return ""
		}
		auditLog := mailMgr.GetAuditLog()
		if auditLog == nil {
			return ""
		}
		return auditLog.Read()
	}

	if path == "/status" {
		var lines []string
		mailMgr := s.mgr.GetMailManager()
		for _, id := range s.mgr.List() {
			sess := s.mgr.Get(id)
			if sess != nil {
				state := sess.State()
				idleSince := "-"
				inboxCount := 0
				
				if tmuxSess, ok := sess.(*tmux.Session); ok {
					if state == "idle" {
						idleDuration := tmuxSess.IdleDuration()
						if idleDuration > 0 {
							idleSince = fmt.Sprintf("%ds", int(idleDuration.Seconds()))
						}
					}
				}
				
				if mailMgr != nil {
					inboxCount = len(mailMgr.GetInbox(id))
				}
				
				lines = append(lines, fmt.Sprintf("%s %s %s %d", id, state, idleSince, inboxCount))
			}
		}
		return strings.Join(lines, "\n") + "\n"
	}

	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	
	// User message file: /user/mailbox/msg-*.json
	if len(parts) == 3 && parts[0] == "user" && (parts[1] == "inbox" || parts[1] == "outbox" || parts[1] == "completed") {
		msgFile := parts[2]
		msgID := strings.TrimSuffix(msgFile, ".json")
		
		mailMgr := s.mgr.GetMailManager()
		if mailMgr == nil {
			return ""
		}
		
		msg, err := mailMgr.GetMessage("user", msgID)
		if err != nil {
			return ""
		}
		
		data, _ := msg.ToJSON()
		return string(data)
	}
	
	// Message file: /sessID/mailbox/msg-*.json
	if len(parts) == 3 && (parts[1] == "inbox" || parts[1] == "outbox" || parts[1] == "completed") {
		sessID := parts[0]
		_ = parts[1] // mailboxType (not used, just for validation)
		msgFile := parts[2]
		msgID := strings.TrimSuffix(msgFile, ".json")
		
		mailMgr := s.mgr.GetMailManager()
		if mailMgr == nil {
			return ""
		}
		
		msg, err := mailMgr.GetMessage(sessID, msgID)
		if err != nil {
			return ""
		}
		
		data, _ := msg.ToJSON()
		return string(data)
	}
	
	// Regular session file
	if len(parts) != 2 {
		return ""
	}
	sess := s.mgr.Get(parts[0])
	if sess == nil {
		return ""
	}
	return s.getSessionFile(sess, fileIndex(parts[1]))
}

func (s *Server) getSessionFile(sess backend.Session, idx int) string {
	meta := sess.Metadata()

	switch idx {
	case fileState:
		return sess.State()
	case filePid:
		if meta.Pid == 0 {
			return ""
		}
		return strconv.Itoa(meta.Pid)
	case fileCwd:
		return meta.Cwd
	case fileAlias:
		if meta.Alias == "" {
			return sess.ID()
		}
		return meta.Alias
	case fileBackend:
		return meta.Backend
	case fileContext:
		if tmuxSess, ok := sess.(*tmux.Session); ok {
			return tmuxSess.GetContext()
		}
		return ""
	case fileRole:
		return sess.Role()
	case fileTasks:
		return strings.Join(sess.Tasks(), ",")
	case fileTmux:
		tmuxSession, hasSession := meta.Extra["tmux_session"]
		tmuxWindow, hasWindow := meta.Extra["tmux_window"]
		if hasSession && hasWindow {
			return fmt.Sprintf("%s:%s", tmuxSession, tmuxWindow)
		}
		return ""
	}
	return ""
}

func (s *Server) pathToDir(path string, qid plan9.Qid) plan9.Dir {
	name := filepath.Base(path)
	if path == "/" {
		name = "."
	}
	mode := uint32(0444)
	if qid.Type&QTDir != 0 {
		mode = plan9.DMDIR | 0555
	}
	return plan9.Dir{Qid: qid, Mode: plan9.Perm(mode), Name: name, Uid: "q", Gid: "q", Muid: "q"}
}

func (s *Server) Close() error {
	return s.listener.Close()
}

func errFcall(fc *plan9.Fcall, msg string) *plan9.Fcall {
	return &plan9.Fcall{Type: plan9.Rerror, Tag: fc.Tag, Ename: msg}
}

func fileIndex(name string) int {
	for i, n := range fileNames {
		if n == name {
			return i
		}
	}
	return -1
}

func hashID(id string) uint64 {
	var h uint64 = 5381
	for _, c := range id {
		h = ((h << 5) + h) + uint64(c)
	}
	return h
}
