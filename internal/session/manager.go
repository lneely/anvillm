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
	backends      map[string]backend.Backend
	sessions      map[string]backend.Session
	mailManager   *mailbox.Manager
	OnStateChange func(sessionID, oldState, newState string)
	mu            sync.RWMutex
	stopCh        chan struct{}
	wg            sync.WaitGroup
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
	
	// Set session getter for alias lookup
	mailMgr.SetSessionGetter(m)
	
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

	// Wire up state change callback
	if tmuxSess, ok := sess.(*tmux.Session); ok && m.OnStateChange != nil {
		tmuxSess.OnStateChange = m.OnStateChange
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

// GetAlias returns the alias for a session ID, or empty string if not set
func (m *Manager) GetAlias(id string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	sess := m.sessions[id]
	if sess == nil {
		return ""
	}
	
	return sess.Metadata().Alias
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
			msg, err := m.mailManager.PeekOutbox(senderID)
			if err != nil {
				break
			}
			// Deliver first, then remove only if successful
			if err := m.mailManager.DeliverToInbox(msg.To, msg); err != nil {
				// Receiver doesn't exist - move to dead letter (completed)
				m.mailManager.RemoveFromOutbox(senderID)
				if msg.Metadata == nil {
					msg.Metadata = make(map[string]interface{})
				}
				msg.Metadata["error"] = err.Error()
				m.mailManager.MoveToCompleted(senderID, msg)
				fmt.Fprintf(os.Stderr, "Message undeliverable to %s: %v\n", msg.To, err)
			} else {
				// Remove only after successful delivery
				m.mailManager.RemoveFromOutbox(senderID)
			}
		}
	}
	
	// 2. Process user inbox with type-based routing
	userMessages, _ := m.mailManager.GetPendingMessages("user")
	for _, msg := range userMessages {
		// Route based on message type
		switch msg.Type {
		case mailbox.MessageTypePromptResponse:
			// Auto-complete
			m.mailManager.CompleteMessage("user", msg.ID)

		default:
			// All other types: inbox only, DON'T auto-complete
			// Message stays in inbox for user to review
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
		sess.Send(ctx, fmt.Sprintf("You have a new message. Read it using read_inbox (agent_id=%s) and respond appropriately.", sess.ID()))
	}
}

// GetMailManager returns the mailbox manager
func (m *Manager) GetMailManager() *mailbox.Manager {
	return m.mailManager
}
