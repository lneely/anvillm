// Package p9 implements the 9P filesystem for Q agent sessions.
package p9

import (
	"anvillm/internal/backend"
	"anvillm/internal/backend/tmux"
	"anvillm/internal/session"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"9fans.net/go/plan9"
	"9fans.net/go/plan9/client"
)

/*
Filesystem layout:

agent/
    ctl                 (write) "new <backend> <cwd>" creates session, returns id
    list                (read)  list sessions: "id alias state pid cwd"
    {session-id}/
        ctl             (write) "stop", "restart", "kill", "refresh"
        in              (write) send prompt (non-blocking, validates and returns immediately)
        out             (read)  response from last prompt
        state           (read)  "starting", "idle", "running", "stopped", "error", "exited"
        pid             (read)  process id
        cwd             (read)  working directory
        alias           (r/w)   session alias
        backend         (read)  backend name (e.g., "kiro-cli", "claude")
        context         (r/w)   text prepended to every prompt

Bot-to-bot communication:
    Any bot can discover peers via agent/list (shows id, alias, state).
    To talk to a peer: write prompt to agent/{peer-id}/in, read agent/{peer-id}/out.
    Use aliases (e.g., "reviewer", "dev") so bots can find each other by role.
    Set context to inject peer awareness: echo "You are dev. Peer reviewer is at agent/abc123" > agent/{id}/context
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
	qidSessionBase = 1000
	qidPeersBase   = 0x10000000 // peers/{id}/file
)

// File indices within a session directory
const (
	fileCtl = iota
	fileIn
	fileOut
	fileState
	filePid
	fileCwd
	fileAlias
	fileBackend
	fileContext
	fileCount
)

var fileNames = []string{"ctl", "in", "out", "state", "pid", "cwd", "alias", "backend", "context"}

type Server struct {
	mgr        *session.Manager
	listener   net.Listener
	socketPath string
	mu         sync.RWMutex
}

type connState struct {
	fids map[uint32]*fid
	mu   sync.RWMutex
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

	s := &Server{mgr: mgr, listener: listener, socketPath: sockPath}
	go s.acceptLoop()
	return s, nil
}

// SocketPath returns the path to the Unix socket
func (s *Server) SocketPath() string {
	return s.socketPath
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
	case plan9.Tread:
		return s.read(cs, fc)
	case plan9.Twrite:
		return s.write(cs, fc)
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
			default:
				// Check if it's a session ID
				if sess := s.mgr.Get(name); sess != nil {
					qid = plan9.Qid{Type: QTDir, Path: qidSessionBase + hashID(name)}
					newPath = "/" + name
				} else {
					return errFcall(fc, "not found")
				}
			}
		} else if strings.Count(path, "/") == 1 && path != "/" {
			// Inside a session directory
			sessID := strings.TrimPrefix(path, "/")
			if s.mgr.Get(sessID) == nil {
				return errFcall(fc, "session not found")
			}
			idx := fileIndex(name)
			if idx < 0 {
				return errFcall(fc, "not found")
			}
			qid = plan9.Qid{Type: QTFile, Path: qidSessionBase + hashID(sessID)*fileCount + uint64(idx)}
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
	parts := strings.SplitN(f.path, "/", 3)

	// /ctl - create new session
	if f.path == "/ctl" {
		args := strings.Fields(input)
		fmt.Fprintf(os.Stderr, "[DEBUG] ctl write: input=%q args=%v len=%d fc.Count=%d len(fc.Data)=%d\n",
			input, args, len(args), fc.Count, len(fc.Data))
		if len(args) < 2 || args[0] != "new" {
			return errFcall(fc, "usage: new <backend> <cwd>")
		}
		backendName := args[1]
		cwd, _ := os.Getwd()
		if len(args) > 2 {
			cwd = strings.Trim(args[2], `"`)
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

		_, err := s.mgr.New(backendName, cleanPath)
		if err != nil {
			return errFcall(fc, err.Error())
		}
		fmt.Fprintf(os.Stderr, "[DEBUG] ctl write: session created, returning Count=%d\n", uint32(len(fc.Data)))
		return &plan9.Fcall{Type: plan9.Rwrite, Tag: fc.Tag, Count: uint32(len(fc.Data))}
	}

	// /{id}/ctl - session control
	if len(parts) == 3 && parts[2] == "ctl" {
		sess := s.mgr.Get(parts[1])
		if sess == nil {
			return errFcall(fc, "session not found")
		}
		args := strings.Fields(input)
		if len(args) == 0 {
			return errFcall(fc, "usage: stop | restart | kill | refresh")
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
		default:
			return errFcall(fc, "unknown command")
		}
		return &plan9.Fcall{Type: plan9.Rwrite, Tag: fc.Tag, Count: uint32(len(fc.Data))}
	}

	// /{id}/in - send prompt (async, non-blocking)
	if len(parts) == 3 && parts[2] == "in" {
		sess := s.mgr.Get(parts[1])
		if sess == nil {
			return errFcall(fc, "session not found")
		}
		fmt.Fprintf(os.Stderr, "[DEBUG] /in write: session=%s input=%q state=%s\n", parts[1], input, sess.State())
		// Use async send to avoid blocking on response
		if tmuxSess, ok := sess.(*tmux.Session); ok {
			ctx := context.Background()
			if err := tmuxSess.SendAsync(ctx, input); err != nil {
				fmt.Fprintf(os.Stderr, "[DEBUG] /in write: SendAsync failed: %v\n", err)
				return errFcall(fc, err.Error())
			}
			fmt.Fprintf(os.Stderr, "[DEBUG] /in write: SendAsync succeeded\n")
		} else {
			return errFcall(fc, "async send not supported for this backend")
		}
		return &plan9.Fcall{Type: plan9.Rwrite, Tag: fc.Tag, Count: uint32(len(fc.Data))}
	}

	// /{id}/alias - set session alias
	if len(parts) == 3 && parts[2] == "alias" {
		sess := s.mgr.Get(parts[1])
		if sess == nil {
			return errFcall(fc, "session not found")
		}
		// Validate alias: alphanumeric, hyphen, underscore only
		matched, _ := regexp.MatchString(`^[A-Za-z0-9_-]+$`, input)
		if !matched {
			return errFcall(fc, "invalid alias: must match [A-Za-z0-9_-]+")
		}
		sess.SetAlias(input)
		return &plan9.Fcall{Type: plan9.Rwrite, Tag: fc.Tag, Count: uint32(len(fc.Data))}
	}

	// /{id}/context - set context prefix
	if len(parts) == 3 && parts[2] == "context" {
		sess := s.mgr.Get(parts[1])
		if sess == nil {
			return errFcall(fc, "session not found")
		}
		if tmuxSess, ok := sess.(*tmux.Session); ok {
			tmuxSess.SetContext(input)
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
		for _, id := range s.mgr.List() {
			dirs = append(dirs, plan9.Dir{
				Qid:  plan9.Qid{Type: QTDir, Path: qidSessionBase + hashID(id)},
				Mode: plan9.DMDIR | 0555, Name: id, Uid: "q", Gid: "q", Muid: "q",
			})
		}
	} else {
		sessID := strings.TrimPrefix(path, "/")
		sess := s.mgr.Get(sessID)
		if sess == nil {
			return nil
		}
		for i, name := range fileNames {
			mode := uint32(0444)
			if name == "ctl" || name == "in" {
				mode = 0222
			} else if name == "alias" || name == "context" {
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
				lines = append(lines, fmt.Sprintf("%s\t%s\t%s\t%d\t%s", sess.ID(), alias, sess.State(), meta.Pid, meta.Cwd))
			}
		}
		return strings.Join(lines, "\n") + "\n"
	}

	parts := strings.SplitN(path, "/", 3)
	if len(parts) != 3 {
		return ""
	}
	sess := s.mgr.Get(parts[1])
	if sess == nil {
		return ""
	}
	return s.getSessionFile(sess, fileIndex(parts[2]))
}

func (s *Server) getSessionFile(sess backend.Session, idx int) string {
	meta := sess.Metadata()

	switch idx {
	case fileOut:
		// Output from last command - only available for tmux sessions
		if tmuxSess, ok := sess.(*tmux.Session); ok {
			// We don't have a direct Output() method anymore
			// This would need to be stored separately if needed
			_ = tmuxSess
		}
		return ""
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
