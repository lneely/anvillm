// Package p9 implements the 9P filesystem for Q agent sessions.
package p9

import (
	"anvillm/internal/backend"
	"anvillm/internal/backend/tmux"
	"anvillm/internal/eventbus"
	"anvillm/internal/logging"
	"anvillm/internal/mailbox"
	"anvillm/internal/session"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/google/uuid"
	"go.uber.org/zap"

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
        backend         (read)  backend name (e.g., "kiro-cli", "claude", "ollama")
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
	qidEvents                    // agent/events
	qidUser                      // user directory
	qidUserInbox                 // user/inbox
	qidUserOutbox                // user/outbox
	qidUserCompleted             // user/completed
	qidUserCtl                   // user/ctl
	qidUserMail                  // user/mail
	qidBeads                     // beads directory
	qidBeadsCtl                  // beads/ctl
	qidBeadsList                 // beads/list
	qidBeadsMtab                 // beads/mtab
	qidBeadsReady                // beads/ready
	qidTools                     // tools directory
	qidSkills                    // skills directory
	qidRoles                     // roles directory
	qidSessionBase   = 1000
	qidPeersBase     = 0x10000000 // peers/{id}/file
	qidInboxBase     = 0x20000000 // session/{id}/inbox
	qidOutboxBase    = 0x30000000 // session/{id}/outbox
	qidCompletedBase = 0x40000000 // session/{id}/completed
	qidMessageBase   = 0x50000000 // message files
	qidBeadsBase     = 0x60000000 // beads/{id}/file
	qidToolsBase     = 0x70000000 // tools/{capability}/{tool}
	qidSkillsBase    = 0x80000000 // skills/{intent}/{skill}
	qidRolesBase     = 0x90000000 // roles/{focus-area}/{role}
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
	fileSandbox
	fileTmux
	fileMail
	fileModel
	fileRole
	fileCount
)

var fileNames = []string{"ctl", "state", "pid", "cwd", "alias", "backend", "context", "sandbox", "tmux", "mail", "model", "role"}

// Directory names in session
var dirNames = []string{"inbox", "outbox", "completed"}

// Server implements a 9P file server for agent session management.
// It exposes sessions, beads, tools, skills, and events through a virtual filesystem.
type Server struct {
	mgr           *session.Manager
	listener      net.Listener
	socketPath    string
	events        *eventbus.Bus
	beads         *BeadsFS
	tools         *ToolsFS
	skills        *SkillsFS
	roles         *RolesFS
	OnAliasChange func(backend.Session) // Called when session alias changes
	mu            sync.RWMutex
}

type connState struct {
	fids      map[uint32]*fid
	mu        sync.RWMutex
	sessionID string // Track which session owns this connection
}

type fid struct {
	qid        plan9.Qid
	path       string
	mode       uint8
	offset     int64
	// For streaming /events endpoint
	eventCh    <-chan *eventbus.Event
	eventUnsub func()
}

// NewServer creates and starts the 9P server.
func NewServer(mgr *session.Manager, beadsFS *BeadsFS) (*Server, error) {
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

	// Load tools from anvilmcp
	tools := loadMCPTools()
	toolsFS := NewToolsFS(tools)

	s := &Server{
		mgr:        mgr,
		listener:   listener,
		socketPath: sockPath,
		events:     eventbus.New(),
		beads:      beadsFS,
		tools:      toolsFS,
		skills:     NewSkillsFS(),
		roles:      NewRolesFS(),
	}
	go s.acceptLoop()
	return s, nil
}

func loadMCPTools() []Tool {
	// These tool definitions are exposed via 9P at agent/tools/anvilmcp/
	// for progressive discovery by agents using the code execution pattern.
	// The actual MCP tool (execute_code) is defined in main.go for tools/list.
	return []Tool{
		{
			Name:        "read_inbox",
			Description: "Read messages from agent's inbox",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"agent_id": {Type: "string", Description: "Agent session ID (or 'user')"},
				},
				Required: []string{"agent_id"},
			},
		},
		{
			Name:        "send_message",
			Description: "Send message to another agent or user",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"from":    {Type: "string", Description: "Sender agent ID (or 'user')"},
					"to":      {Type: "string", Description: "Recipient agent ID (or 'user')"},
					"type":    {Type: "string", Description: "Message type"},
					"subject": {Type: "string", Description: "Message subject"},
					"body":    {Type: "string", Description: "Message body"},
				},
				Required: []string{"from", "to", "type", "subject", "body"},
			},
		},
		{
			Name:        "list_sessions",
			Description: "List all active sessions",
			InputSchema: InputSchema{
				Type:       "object",
				Properties: map[string]Property{},
			},
		},
		{
			Name:        "set_state",
			Description: "Set agent state (idle, running, stopped, etc.)",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"agent_id": {Type: "string", Description: "Agent session ID"},
					"state":    {Type: "string", Description: "State value", Enum: []string{"idle", "running", "stopped", "starting", "error", "exited"}},
				},
				Required: []string{"agent_id", "state"},
			},
		},
		{
			Name:        "list_skills",
			Description: "List all available skills from $ANVILLM_SKILLS_PATH",
			InputSchema: InputSchema{
				Type:       "object",
				Properties: map[string]Property{},
			},
		},
		{
			Name:        "execute_code",
			Description: "Execute bash code as a subprocess with timeout",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"code":     {Type: "string", Description: "Bash code to execute"},
					"language": {Type: "string", Description: "Programming language (bash)", Enum: []string{"bash"}},
					"timeout":  {Type: "integer", Description: "Timeout in seconds (default: 30)"},
				},
				Required: []string{"code"},
			},
		},
	}
}

// SocketPath returns the path to the Unix socket
func (s *Server) SocketPath() string {
	return s.socketPath
}

// Events returns the event bus for publishing events.
func (s *Server) Events() *eventbus.Bus {
	return s.events
}

// Beads returns the BeadsFS instance.
func (s *Server) Beads() *BeadsFS {
	return s.beads
}

func (s *Server) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				logging.Logger().Error("accept error", zap.Error(err))
			}
			return
		}
		go s.serve(conn)
	}
}

func (s *Server) serve(conn net.Conn) {
	defer conn.Close()
	cs := &connState{fids: make(map[uint32]*fid)}

	// Cancel any outstanding event subscriptions when the connection drops.
	defer func() {
		cs.mu.Lock()
		defer cs.mu.Unlock()
		for _, f := range cs.fids {
			if f.eventUnsub != nil {
				f.eventUnsub()
				f.eventUnsub = nil
			}
		}
	}()

	for {
		fc, err := plan9.ReadFcall(conn)
		if err != nil {
			if err != io.EOF {
				logging.Logger().Error("read error", zap.Error(err))
			}
			return
		}

		rfc := s.handle(cs, fc)
		if err := plan9.WriteFcall(conn, rfc); err != nil {
			logging.Logger().Error("write error", zap.Error(err))
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
		if f, ok := cs.fids[fc.Fid]; ok && f.eventUnsub != nil {
			// Cancel event subscription before dropping the fid.
			f.eventUnsub()
			f.eventUnsub = nil
		}
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
			case "events":
				qid = plan9.Qid{Type: QTFile, Path: qidEvents}
				newPath = "/events"
			case "user":
				qid = plan9.Qid{Type: QTDir, Path: qidUser}
				newPath = "/user"
			case "beads":
				qid = plan9.Qid{Type: QTDir, Path: qidBeads}
				newPath = "/beads"
			case "tools":
				qid = plan9.Qid{Type: QTDir, Path: qidTools}
				newPath = "/tools"
			case "skills":
				qid = plan9.Qid{Type: QTDir, Path: qidSkills}
				newPath = "/skills"
			case "roles":
				qid = plan9.Qid{Type: QTDir, Path: qidRoles}
				newPath = "/roles"
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
		} else if path == "/beads" {
			// Inside beads directory
			switch name {
			case "ctl":
				qid = plan9.Qid{Type: QTFile, Path: qidBeadsCtl}
				newPath = "/beads/ctl"
			case "list":
				qid = plan9.Qid{Type: QTFile, Path: qidBeadsList}
				newPath = "/beads/list"
			case "mtab":
				qid = plan9.Qid{Type: QTFile, Path: qidBeadsMtab}
				newPath = "/beads/mtab"
			case "ready":
				qid = plan9.Qid{Type: QTFile, Path: qidBeadsReady}
				newPath = "/beads/ready"
			default:
				// Mount directory - verify it exists
				if s.beads != nil {
					mounts := s.beads.ListMounts()
					if _, isMount := mounts[name]; isMount {
						qid = plan9.Qid{Type: QTDir, Path: qidBeadsBase + hashID(name)}
						newPath = "/beads/" + name
					} else {
						return errFcall(fc, "not found")
					}
				} else {
					return errFcall(fc, "not found")
				}
			}
		} else if path == "/tools" {
			// Inside tools directory - list capabilities + help
			if s.tools != nil {
				caps, _ := s.tools.listCapabilities()
				// help file
				qid = plan9.Qid{Type: QTFile, Path: qidToolsBase}
				if name == "help" {
					newPath = "/tools/help"
				} else if slices.Contains(caps, name) {
					qid = plan9.Qid{Type: QTDir, Path: qidToolsBase + hashID(name)}
					newPath = "/tools/" + name
				} else {
					return errFcall(fc, "not found")
				}
			} else {
				return errFcall(fc, "not found")
			}
		} else if strings.HasPrefix(path, "/tools/") && strings.Count(path, "/") == 2 {
			// Inside tools/<capability> - tool files
			qid = plan9.Qid{Type: QTFile, Path: qidToolsBase + hashID(path+name)}
			newPath = path + "/" + name
		} else if path == "/skills" {
			// Inside skills directory - list intents + help
			if s.skills != nil {
				intents, _ := s.skills.listIntents()
				qid = plan9.Qid{Type: QTFile, Path: qidSkillsBase}
				if name == "help" {
					newPath = "/skills/help"
				} else if slices.Contains(intents, name) {
					qid = plan9.Qid{Type: QTDir, Path: qidSkillsBase + hashID(name)}
					newPath = "/skills/" + name
				} else {
					return errFcall(fc, "not found")
				}
			} else {
				return errFcall(fc, "not found")
			}
		} else if strings.HasPrefix(path, "/skills/") && strings.Count(path, "/") == 2 {
			// Inside skills/<intent> - skill directories
			qid = plan9.Qid{Type: QTDir, Path: qidSkillsBase + hashID(path+name)}
			newPath = path + "/" + name
		} else if strings.HasPrefix(path, "/skills/") && strings.Count(path, "/") == 3 {
			// Inside skills/<intent>/<skill> - files
			qid = plan9.Qid{Type: QTFile, Path: qidSkillsBase + hashID(path+name)}
			newPath = path + "/" + name
		} else if path == "/roles" {
			// Inside roles directory - list focus areas + help
			if s.roles != nil {
				focusAreas, _ := s.roles.listFocusAreas()
				qid = plan9.Qid{Type: QTFile, Path: qidRolesBase}
				if name == "help" {
					newPath = "/roles/help"
				} else if slices.Contains(focusAreas, name) {
					qid = plan9.Qid{Type: QTDir, Path: qidRolesBase + hashID(name)}
					newPath = "/roles/" + name
				} else {
					return errFcall(fc, "not found")
				}
			} else {
				return errFcall(fc, "not found")
			}
		} else if strings.HasPrefix(path, "/roles/") && strings.Count(path, "/") == 2 {
			// Inside roles/<focus-area> - role files
			qid = plan9.Qid{Type: QTFile, Path: qidRolesBase + hashID(path+name)}
			newPath = path + "/" + name
		} else if strings.HasPrefix(path, "/beads/") && strings.Count(path, "/") == 2 {
			// Inside a mount directory
			mountName := strings.TrimPrefix(path, "/beads/")
			if s.beads != nil {
				mounts := s.beads.ListMounts()
				if _, isMount := mounts[mountName]; isMount {
					// Check if name is a control file or bead ID
					switch name {
					case "cwd", "ctl", "list", "ready", "pending", "stats", "blocked", "stale", "query", "config":
						qid = plan9.Qid{Type: QTFile, Path: qidBeadsBase + hashID(mountName+name)}
						newPath = path + "/" + name
					default:
						// Bead ID directory
						qid = plan9.Qid{Type: QTDir, Path: qidBeadsBase + hashID(mountName+name)}
						newPath = path + "/" + name
					}
				} else {
					return errFcall(fc, "not found")
				}
			} else {
				return errFcall(fc, "not found")
			}
		} else if strings.HasPrefix(path, "/beads/") && strings.Count(path, "/") == 3 {
			// Inside a mount's bead directory - property files
			qid = plan9.Qid{Type: QTFile, Path: qidBeadsBase + hashID(path+name)}
			newPath = path + "/" + name
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
			switch name {
			case "inbox":
				qid = plan9.Qid{Type: QTDir, Path: qidInboxBase + hashID(sessID)}
				newPath = path + "/inbox"
			case "outbox":
				qid = plan9.Qid{Type: QTDir, Path: qidOutboxBase + hashID(sessID)}
				newPath = path + "/outbox"
			case "completed":
				qid = plan9.Qid{Type: QTDir, Path: qidCompletedBase + hashID(sessID)}
				newPath = path + "/completed"
			default:
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

	if len(qids) == 0 {
		return errFcall(fc, "walk failed")
	}

	cs.fids[fc.Newfid] = &fid{qid: qids[len(qids)-1], path: path}
	return &plan9.Fcall{Type: plan9.Rwalk, Tag: fc.Tag, Wqid: qids}
}

func (s *Server) open(cs *connState, fc *plan9.Fcall) *plan9.Fcall {
	cs.mu.Lock()
	f, ok := cs.fids[fc.Fid]
	if !ok {
		cs.mu.Unlock()
		return errFcall(fc, "bad fid")
	}
	f.mode = fc.Mode
	f.offset = 0

	// For the /events streaming endpoint, subscribe to the event bus.
	if f.path == "/events" {
		ch, cancel := s.events.Subscribe()
		f.eventCh = ch
		f.eventUnsub = cancel
	}
	qid := f.qid
	cs.mu.Unlock()

	return &plan9.Fcall{Type: plan9.Ropen, Tag: fc.Tag, Qid: qid}
}

func (s *Server) create(_ *connState, fc *plan9.Fcall) *plan9.Fcall {
	return errFcall(fc, "create not supported")
}

func (s *Server) read(cs *connState, fc *plan9.Fcall) *plan9.Fcall {
	cs.mu.RLock()
	f, ok := cs.fids[fc.Fid]
	if !ok {
		cs.mu.RUnlock()
		return errFcall(fc, "bad fid")
	}

	// Streaming /events: block until next event arrives (or channel closed).
	if f.eventCh != nil {
		cs.mu.RUnlock()
		e, ok := <-f.eventCh
		if !ok {
			// Channel closed (subscription cancelled); signal EOF.
			return &plan9.Fcall{Type: plan9.Rread, Tag: fc.Tag, Count: 0}
		}
		data := eventbus.MarshalEvent(e)
		return &plan9.Fcall{Type: plan9.Rread, Tag: fc.Tag, Count: uint32(len(data)), Data: data}
	}

	path := f.path
	isDir := f.qid.Type&QTDir != 0
	cs.mu.RUnlock()

	var data []byte

	if isDir {
		data = s.readDir(path, fc.Offset, fc.Count)
	} else {
		content := s.readFile(path)
		if fc.Offset < uint64(len(content)) {
			end := min(int(fc.Offset)+int(fc.Count), len(content))
			data = []byte(content[fc.Offset:end])
		}
	}

	return &plan9.Fcall{Type: plan9.Rread, Tag: fc.Tag, Count: uint32(len(data)), Data: data}
}

func (s *Server) write(cs *connState, fc *plan9.Fcall) *plan9.Fcall {
	cs.mu.RLock()
	f, ok := cs.fids[fc.Fid]
	if !ok {
		cs.mu.RUnlock()
		return errFcall(fc, "bad fid")
	}
	path := f.path
	cs.mu.RUnlock()

	input := strings.TrimSpace(string(fc.Data))
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")

	// Handle beads writes
	if beadsPath, ok := strings.CutPrefix(path, "/beads/"); ok {
		if s.beads != nil {
			cs.mu.Lock()
			sessionID := cs.sessionID
			cs.mu.Unlock()
			if err := s.beads.Write(beadsPath, fc.Data, sessionID); err != nil {
				return errFcall(fc, err.Error())
			}
			return &plan9.Fcall{Type: plan9.Rwrite, Tag: fc.Tag, Count: uint32(len(fc.Data))}
		}
		return errFcall(fc, "beads not initialized")
	}

	// /ctl - create new session
	if path == "/ctl" {
		args := strings.Fields(input)
		if len(args) < 2 || args[0] != "new" {
			return errFcall(fc, "usage: new <backend> <cwd> [sandbox=<sandbox>] [model=<model>]")
		}

		backendName := args[1]
		cwd, err := os.Getwd()
		if err != nil {
			return errFcall(fc, fmt.Sprintf("failed to get working directory: %v", err))
		}
		var sbx string
		var model string

		// Parse remaining arguments: first non-key=value is cwd, rest are options
		cwdSet := false
		for i := 2; i < len(args); i++ {
			arg := args[i]
			if s, ok := strings.CutPrefix(arg, "sandbox="); ok {
				sbx = s
			} else if m, ok := strings.CutPrefix(arg, "model="); ok {
				model = m
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
			CWD:     cleanPath,
			Sandbox: sbx,
			Model:   model,
		}
		_, err = s.mgr.New(opts, backendName)
		if err != nil {
			return errFcall(fc, err.Error())
		}
		return &plan9.Fcall{Type: plan9.Rwrite, Tag: fc.Tag, Count: uint32(len(fc.Data))}
	}

	// /{id}/ctl - session control
	if len(parts) == 2 && parts[1] == "ctl" {
		// Handle user/ctl specially
		if parts[0] == "user" {
			args := strings.Fields(input)
			if len(args) == 0 {
				return errFcall(fc, "usage: complete <msg-id> | delete <msg-id>")
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
			case "delete":
				if len(args) < 2 {
					return errFcall(fc, "usage: delete <msg-id>")
				}
				msgID := args[1]
				mailMgr := s.mgr.GetMailManager()
				if err := mailMgr.DeleteFromCompleted("user", msgID); err != nil {
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
		case "clear":
			if tmuxSess, ok := sess.(*tmux.Session); ok {
				if err := tmuxSess.Clear(); err != nil {
					return errFcall(fc, err.Error())
				}
			}
		case "compact":
			if tmuxSess, ok := sess.(*tmux.Session); ok {
				if err := tmuxSess.Compact(); err != nil {
					return errFcall(fc, err.Error())
				}
			}
		case "resume":
			if tmuxSess, ok := sess.(*tmux.Session); ok {
				if err := tmuxSess.Resume(); err != nil {
					return errFcall(fc, err.Error())
				}
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

	// /{id}/role - set role and auto-load from RolesFS
	if len(parts) == 2 && parts[1] == "role" {
		sess := s.mgr.Get(parts[0])
		if sess == nil {
			return errFcall(fc, "session not found")
		}
		if tmuxSess, ok := sess.(*tmux.Session); ok {
			tmuxSess.SetRole(input)
			// Auto-load role definition from RolesFS
			if s.roles != nil {
				roleContent, err := s.roles.ReadRole(input)
				if err != nil {
					logging.Logger().Warn("role not found", zap.String("role", input), zap.Error(err))
				} else {
					tmuxSess.SetContext(roleContent)
				}
			}
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

		// Track this connection's session ID (first write wins)
		cs.mu.Lock()
		cs.sessionID = sessID
		cs.mu.Unlock()

		return &plan9.Fcall{Type: plan9.Rwrite, Tag: fc.Tag, Count: uint32(len(fc.Data))}
	}

	// /{id}/mail or /user/mail - write message to outbox
	if len(parts) == 2 && parts[1] == "mail" {
		sessID := parts[0]

		// Track this connection's session ID (first write wins)
		cs.mu.Lock()
		cs.sessionID = sessID
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

		// Set from field
		msg.From = sessID

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

		// Transition sender to idle after sending (only for non-user sessions)
		if sessID != "user" {
			if sess := s.mgr.Get(sessID); sess != nil {
				if tmuxSess, ok := sess.(*tmux.Session); ok {
					tmuxSess.TransitionTo("idle")
				}
			}
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
	if !ok {
		cs.mu.Unlock()
		return errFcall(fc, "bad fid")
	}
	path := f.path
	cs.mu.Unlock()

	cs.mu.RLock()
	sessionID := cs.sessionID
	cs.mu.RUnlock()

	// Only support removing inbox messages (marks them as completed)
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) == 3 && parts[1] == "inbox" && strings.HasSuffix(parts[2], ".json") {
		sessID := parts[0]
		msgID := strings.TrimSuffix(parts[2], ".json")

		// Validate ownership: only allow removing from own inbox or user inbox
		if sessID != "user" && sessID != sessionID {
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
			Qid: plan9.Qid{Type: QTFile, Path: qidEvents}, Mode: 0644, Name: "events",
			Uid: "q", Gid: "q", Muid: "q",
		})
		dirs = append(dirs, plan9.Dir{
			Qid:  plan9.Qid{Type: QTDir, Path: qidUser},
			Mode: plan9.DMDIR | 0555, Name: "user", Uid: "q", Gid: "q", Muid: "q",
		})
		dirs = append(dirs, plan9.Dir{
			Qid:  plan9.Qid{Type: QTDir, Path: qidBeads},
			Mode: plan9.DMDIR | 0555, Name: "beads", Uid: "q", Gid: "q", Muid: "q",
		})
		dirs = append(dirs, plan9.Dir{
			Qid:  plan9.Qid{Type: QTDir, Path: qidTools},
			Mode: plan9.DMDIR | 0555, Name: "tools", Uid: "q", Gid: "q", Muid: "q",
		})
		dirs = append(dirs, plan9.Dir{
			Qid:  plan9.Qid{Type: QTDir, Path: qidSkills},
			Mode: plan9.DMDIR | 0555, Name: "skills", Uid: "q", Gid: "q", Muid: "q",
		})
		dirs = append(dirs, plan9.Dir{
			Qid:  plan9.Qid{Type: QTDir, Path: qidRoles},
			Mode: plan9.DMDIR | 0555, Name: "roles", Uid: "q", Gid: "q", Muid: "q",
		})
		for _, id := range s.mgr.List() {
			dirs = append(dirs, plan9.Dir{
				Qid:  plan9.Qid{Type: QTDir, Path: qidSessionBase + hashID(id)},
				Mode: plan9.DMDIR | 0555, Name: id, Uid: "q", Gid: "q", Muid: "q",
			})
		}
	} else if path == "/tools" {
		if s.tools != nil {
			// Add help file
			dirs = append(dirs, plan9.Dir{
				Qid:  plan9.Qid{Type: QTFile, Path: qidToolsBase},
				Mode: 0444, Name: "help", Uid: "q", Gid: "q", Muid: "q",
			})
			// Add capability directories
			caps, _ := s.tools.listCapabilities()
			for _, cap := range caps {
				dirs = append(dirs, plan9.Dir{
					Qid:  plan9.Qid{Type: QTDir, Path: qidToolsBase + hashID(cap)},
					Mode: plan9.DMDIR | 0555, Name: cap, Uid: "q", Gid: "q", Muid: "q",
				})
			}
		}
	} else if strings.HasPrefix(path, "/tools/") && strings.Count(path, "/") == 2 {
		// Inside tools/<capability> - list tools
		capability := strings.TrimPrefix(path, "/tools/")
		if s.tools != nil {
			tools, _ := s.tools.listToolsInCapability(capability)
			for _, tool := range tools {
				dirs = append(dirs, plan9.Dir{
					Qid:  plan9.Qid{Type: QTFile, Path: qidToolsBase + hashID(path+tool.Name)},
					Mode: 0444, Name: tool.Name, Uid: "q", Gid: "q", Muid: "q",
				})
			}
		}
	} else if path == "/skills" {
		if s.skills != nil {
			// Add help file
			dirs = append(dirs, plan9.Dir{
				Qid:  plan9.Qid{Type: QTFile, Path: qidSkillsBase},
				Mode: 0444, Name: "help", Uid: "q", Gid: "q", Muid: "q",
			})
			// Add intent directories
			intents, _ := s.skills.listIntents()
			for _, intent := range intents {
				dirs = append(dirs, plan9.Dir{
					Qid:  plan9.Qid{Type: QTDir, Path: qidSkillsBase + hashID(intent)},
					Mode: plan9.DMDIR | 0555, Name: intent, Uid: "q", Gid: "q", Muid: "q",
				})
			}
		}
	} else if strings.HasPrefix(path, "/skills/") && strings.Count(path, "/") == 2 {
		// Inside skills/<intent> - list skill directories
		intent := strings.TrimPrefix(path, "/skills/")
		if s.skills != nil {
			skills, _ := s.skills.listSkillsInIntent(intent)
			for _, skill := range skills {
				dirs = append(dirs, plan9.Dir{
					Qid:  plan9.Qid{Type: QTDir, Path: qidSkillsBase + hashID(path+skill.Name)},
					Mode: plan9.DMDIR | 0555, Name: skill.Name, Uid: "q", Gid: "q", Muid: "q",
				})
			}
		}
	} else if strings.HasPrefix(path, "/skills/") && strings.Count(path, "/") == 3 {
		// Inside skills/<intent>/<skill> - list files
		if s.skills != nil {
			skillDirs, _ := s.skills.List("agent" + path)
			for _, sd := range skillDirs {
				dirs = append(dirs, plan9.Dir{
					Qid:  plan9.Qid{Type: sd.Qid.Type, Path: qidSkillsBase + hashID(path+sd.Name)},
					Mode: sd.Mode, Name: sd.Name, Uid: "q", Gid: "q", Muid: "q",
				})
			}
		}
	} else if path == "/roles" {
		if s.roles != nil {
			// Add help file
			dirs = append(dirs, plan9.Dir{
				Qid:  plan9.Qid{Type: QTFile, Path: qidRolesBase},
				Mode: 0444, Name: "help", Uid: "q", Gid: "q", Muid: "q",
			})
			// Add focus area directories
			focusAreas, _ := s.roles.listFocusAreas()
			for _, fa := range focusAreas {
				dirs = append(dirs, plan9.Dir{
					Qid:  plan9.Qid{Type: QTDir, Path: qidRolesBase + hashID(fa)},
					Mode: plan9.DMDIR | 0555, Name: fa, Uid: "q", Gid: "q", Muid: "q",
				})
			}
		}
	} else if strings.HasPrefix(path, "/roles/") && strings.Count(path, "/") == 2 {
		// Inside roles/<focus-area> - list role files
		focusArea := strings.TrimPrefix(path, "/roles/")
		if s.roles != nil {
			roles, _ := s.roles.listRolesInFocusArea(focusArea)
			for _, role := range roles {
				dirs = append(dirs, plan9.Dir{
					Qid:  plan9.Qid{Type: QTFile, Path: qidRolesBase + hashID(path+role.Name)},
					Mode: 0444, Name: role.Name + ".md", Uid: "q", Gid: "q", Muid: "q",
				})
			}
		}
	} else if path == "/beads" {
		// Beads directory - only ctl, mtab, ready (aggregate), and mounted projects
		dirs = append(dirs, plan9.Dir{
			Qid: plan9.Qid{Type: QTFile, Path: qidBeadsCtl}, Mode: 0222, Name: "ctl",
			Uid: "q", Gid: "q", Muid: "q",
		})
		dirs = append(dirs, plan9.Dir{
			Qid: plan9.Qid{Type: QTFile, Path: qidBeadsMtab}, Mode: 0444, Name: "mtab",
			Uid: "q", Gid: "q", Muid: "q",
		})
		dirs = append(dirs, plan9.Dir{
			Qid: plan9.Qid{Type: QTFile, Path: qidBeadsReady}, Mode: 0444, Name: "ready",
			Uid: "q", Gid: "q", Muid: "q",
		})
		// Add mounted projects as directories
		if s.beads != nil {
			for name := range s.beads.ListMounts() {
				dirs = append(dirs, plan9.Dir{
					Qid: plan9.Qid{Type: QTDir, Path: qidBeadsBase + uint64(len(name))}, Mode: 0555 | plan9.DMDIR, Name: name,
					Uid: "q", Gid: "q", Muid: "q",
				})
			}
		}
	} else if strings.HasPrefix(path, "/beads/") && strings.Count(path, "/") == 2 {
		// Inside a mounted project directory - show all beads endpoints
		mountName := strings.TrimPrefix(path, "/beads/")
		if s.beads != nil {
			mounts := s.beads.ListMounts()
			if _, exists := mounts[mountName]; exists {
				// Show all standard beads endpoints for this mount
				dirs = append(dirs, plan9.Dir{
					Qid: plan9.Qid{Type: QTFile, Path: qidBeadsBase + hashID(mountName+"cwd")}, Mode: 0444, Name: "cwd",
					Uid: "q", Gid: "q", Muid: "q",
				})
				dirs = append(dirs, plan9.Dir{
					Qid: plan9.Qid{Type: QTFile, Path: qidBeadsBase + hashID(mountName+"ctl")}, Mode: 0222, Name: "ctl",
					Uid: "q", Gid: "q", Muid: "q",
				})
				dirs = append(dirs, plan9.Dir{
					Qid: plan9.Qid{Type: QTFile, Path: qidBeadsBase + hashID(mountName+"list")}, Mode: 0444, Name: "list",
					Uid: "q", Gid: "q", Muid: "q",
				})
				dirs = append(dirs, plan9.Dir{
					Qid: plan9.Qid{Type: QTFile, Path: qidBeadsBase + hashID(mountName+"ready")}, Mode: 0444, Name: "ready",
					Uid: "q", Gid: "q", Muid: "q",
				})
				dirs = append(dirs, plan9.Dir{
					Qid: plan9.Qid{Type: QTFile, Path: qidBeadsBase + hashID(mountName+"pending")}, Mode: 0444, Name: "pending",
					Uid: "q", Gid: "q", Muid: "q",
				})
				dirs = append(dirs, plan9.Dir{
					Qid: plan9.Qid{Type: QTFile, Path: qidBeadsBase + hashID(mountName+"stats")}, Mode: 0444, Name: "stats",
					Uid: "q", Gid: "q", Muid: "q",
				})
				dirs = append(dirs, plan9.Dir{
					Qid: plan9.Qid{Type: QTFile, Path: qidBeadsBase + hashID(mountName+"blocked")}, Mode: 0444, Name: "blocked",
					Uid: "q", Gid: "q", Muid: "q",
				})
				dirs = append(dirs, plan9.Dir{
					Qid: plan9.Qid{Type: QTFile, Path: qidBeadsBase + hashID(mountName+"stale")}, Mode: 0444, Name: "stale",
					Uid: "q", Gid: "q", Muid: "q",
				})
				dirs = append(dirs, plan9.Dir{
					Qid: plan9.Qid{Type: QTFile, Path: qidBeadsBase + hashID(mountName+"query")}, Mode: 0644, Name: "query",
					Uid: "q", Gid: "q", Muid: "q",
				})
				dirs = append(dirs, plan9.Dir{
					Qid: plan9.Qid{Type: QTFile, Path: qidBeadsBase + hashID(mountName+"config")}, Mode: 0444, Name: "config",
					Uid: "q", Gid: "q", Muid: "q",
				})
				// Add bead directories
				data, err := s.beads.Read(mountName + "/list")
				if err == nil {
					var issues []struct{ ID string `json:"id"` }
					if json.Unmarshal(data, &issues) == nil {
						for _, issue := range issues {
							dirs = append(dirs, plan9.Dir{
								Qid: plan9.Qid{Type: QTDir, Path: qidBeadsBase + hashID(mountName+issue.ID)}, Mode: 0555 | plan9.DMDIR, Name: issue.ID,
								Uid: "q", Gid: "q", Muid: "q",
							})
						}
					}
				}
			}
		}
	} else if strings.HasPrefix(path, "/beads/") && strings.Count(path, "/") == 3 {
		// Inside a mount's bead directory - show property files
		// e.g., /beads/anvillm/anv-77c/json
		// Do NOT show the bead ID itself as a subdirectory
		dirs = append(dirs, plan9.Dir{
			Qid: plan9.Qid{Type: QTFile, Path: qidBeadsBase + hashID(path+"json")}, Mode: 0444, Name: "json",
			Uid: "q", Gid: "q", Muid: "q",
		})
		dirs = append(dirs, plan9.Dir{
			Qid: plan9.Qid{Type: QTFile, Path: qidBeadsBase + hashID(path+"status")}, Mode: 0444, Name: "status",
			Uid: "q", Gid: "q", Muid: "q",
		})
		dirs = append(dirs, plan9.Dir{
			Qid: plan9.Qid{Type: QTFile, Path: qidBeadsBase + hashID(path+"title")}, Mode: 0444, Name: "title",
			Uid: "q", Gid: "q", Muid: "q",
		})
		dirs = append(dirs, plan9.Dir{
			Qid: plan9.Qid{Type: QTFile, Path: qidBeadsBase + hashID(path+"description")}, Mode: 0444, Name: "description",
			Uid: "q", Gid: "q", Muid: "q",
		})
		dirs = append(dirs, plan9.Dir{
			Qid: plan9.Qid{Type: QTFile, Path: qidBeadsBase + hashID(path+"assignee")}, Mode: 0444, Name: "assignee",
			Uid: "q", Gid: "q", Muid: "q",
		})
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
			switch mailboxType {
			case "inbox":
				messages = mailMgr.GetInbox("user")
			case "outbox":
				messages = mailMgr.GetOutbox("user")
			case "completed":
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
			switch name {
			case "ctl", "in", "out":
				mode = 0222
			case "alias", "context", "state":
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
			switch dirName {
			case "inbox":
				qidBase = qidInboxBase
				mode = 0555 // read-only (can list and read files)
			case "outbox":
				qidBase = qidOutboxBase
				mode = 0555 // read-only (can list and read files)
			default:
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
			return []byte("[]")
		}

		var messages []*mailbox.Message
		switch mailboxType {
		case "inbox":
			messages = mailMgr.GetInbox(sessID)
		case "outbox":
			messages = mailMgr.GetOutbox(sessID)
		case "completed":
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

func (s *Server) readFile(path string) string {
	// Handle tools paths
	if strings.HasPrefix(path, "/tools/") {
		if s.tools != nil {
			data, err := s.tools.Read(strings.TrimPrefix(path, "/"))
			if err != nil {
				return ""
			}
			return string(data)
		}
		return ""
	}

	// Handle skills paths
	if strings.HasPrefix(path, "/skills/") {
		if s.skills != nil {
			data, err := s.skills.Read("agent/" + strings.TrimPrefix(path, "/"))
			if err != nil {
				return ""
			}
			return string(data)
		}
		return ""
	}

	// Handle roles paths
	if strings.HasPrefix(path, "/roles/") {
		if s.roles != nil {
			data, err := s.roles.Read("agent/" + strings.TrimPrefix(path, "/"))
			if err != nil {
				return ""
			}
			return string(data)
		}
		return ""
	}

	// Handle beads paths
	if beadsPath, ok := strings.CutPrefix(path, "/beads/"); ok {
		if s.beads != nil {
			data, err := s.beads.Read(beadsPath)
			if err != nil {
				return ""
			}
			return string(data)
		}
		return ""
	}

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
				model := sess.Model()
				if model == "" {
					model = "-"
				}
				lines = append(lines, fmt.Sprintf("%s\t%s\t%s\t%s\t%s\t%s", sess.ID(), backend, sess.State(), alias, model, meta.Cwd))
			}
		}
		return strings.Join(lines, "\n") + "\n"
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
	case fileSandbox:
		return sess.Sandbox()
	case fileTmux:
		tmuxSession, hasSession := meta.Extra["tmux_session"]
		tmuxWindow, hasWindow := meta.Extra["tmux_window"]
		if hasSession && hasWindow {
			return fmt.Sprintf("%s:%s", tmuxSession, tmuxWindow)
		}
		return ""
	case fileModel:
		return sess.Model()
	case fileRole:
		if tmuxSess, ok := sess.(*tmux.Session); ok {
			return tmuxSess.GetRole()
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
