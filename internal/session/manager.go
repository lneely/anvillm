// Package session manages backend sessions
package session

import (
	"anvillm/internal/backend"
	"anvillm/internal/backend/tmux"
	"anvillm/internal/mailbox"
	"context"
	"fmt"
	"os"
	"sync"
	"time"
)

// Manager holds all active sessions
type Manager struct {
	backends    map[string]backend.Backend
	sessions    map[string]backend.Session
	mailManager *mailbox.Manager
	mu          sync.RWMutex
	stopCh      chan struct{}
	wg          sync.WaitGroup
}

// NewManager creates a session manager with the given backends
func NewManager(backends map[string]backend.Backend) *Manager {
	mailMgr := mailbox.NewManager()
	
	m := &Manager{
		backends:    backends,
		sessions:    make(map[string]backend.Session),
		mailManager: mailMgr,
		stopCh:      make(chan struct{}),
	}
	
	// Start mail processing loop
	m.wg.Add(1)
	go m.mailProcessingLoop()
	
	return m
}

// New creates a new session in the given working directory using the specified backend
func (m *Manager) New(opts backend.SessionOptions, backendName string) (backend.Session, error) {
	m.mu.RLock()
	b, ok := m.backends[backendName]
	m.mu.RUnlock()

	if !ok {
		return nil, backend.ErrBackendNotFound
	}

	sess, err := b.CreateSession(context.Background(), opts)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	m.sessions[sess.ID()] = sess
	m.mu.Unlock()

	// Create mailbox structure for new session
	m.mailManager.EnsureMailbox(sess.ID())

	return sess, nil
}

// Get returns a session by ID
func (m *Manager) Get(id string) backend.Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[id]
}

// List returns all session IDs
func (m *Manager) List() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	return ids
}

// Remove removes a session from the manager
func (m *Manager) Remove(id string) {
	m.mu.Lock()
	delete(m.sessions, id)
	m.mu.Unlock()
}

// Stop stops the mail processing loop
func (m *Manager) Stop() {
	close(m.stopCh)
	m.wg.Wait()
}

// mailProcessingLoop processes mailboxes every 5 seconds
func (m *Manager) mailProcessingLoop() {
	defer m.wg.Done()
	
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.processMailboxes()
		}
	}
}

// processMailboxes handles outbox delivery and inbox processing
func (m *Manager) processMailboxes() {
	if m.mailManager == nil {
		return
	}
	
	m.mu.RLock()
	sessions := make([]backend.Session, 0, len(m.sessions))
	for _, sess := range m.sessions {
		sessions = append(sessions, sess)
	}
	m.mu.RUnlock()
	
	// 1. Deliver outbound messages in batch (drain all outboxes)
	allSenders := append([]string{"user"}, m.List()...)
	for _, senderID := range allSenders {
		for m.mailManager.HasOutbox(senderID) {
			msg, err := m.mailManager.ReadOutbox(senderID)
			if err != nil {
				break
			}
			if err := m.mailManager.DeliverToInbox(msg.To, msg); err != nil {
				fmt.Fprintf(os.Stderr, "Error delivering message to %s: %v\n", msg.To, err)
			}
		}
	}
	
	// 2. Process user inbox - write to sender's log and set sender to idle
	userMessages, _ := m.mailManager.GetPendingMessages("user")
	for _, msg := range userMessages {
		// Move to completed
		if err := m.mailManager.CompleteMessage("user", msg.ID); err != nil {
			continue
		}
		
		// Write human-readable output to sender's log and set idle
		if senderSess := m.Get(msg.From); senderSess != nil {
			if tmuxSess, ok := senderSess.(*tmux.Session); ok {
				tmuxSess.AppendToChatLog("ASSISTANT", msg.Body)
				tmuxSess.TransitionTo("idle")
			}
		}
	}
	
	// 3. Prompt idle agents with pending messages (after 15 seconds of idle)
	for _, sess := range sessions {
		if sess.State() != "idle" {
			continue
		}
		
		// Check if session has been idle for more than 15 seconds
		tmuxSess, ok := sess.(*tmux.Session)
		if !ok {
			continue
		}
		
		idleDuration := tmuxSess.IdleDuration()
		if idleDuration < 15*time.Second {
			continue
		}
		
		// Check if inbox has messages
		if !m.mailManager.HasPendingMessages(sess.ID()) {
			continue
		}
		
		// Prompt agent to check inbox
		ctx := context.Background()
		sess.Send(ctx, "check your inbox with 9p-read-inbox")
	}
}

// GetMailManager returns the mailbox manager
func (m *Manager) GetMailManager() *mailbox.Manager {
	return m.mailManager
}
