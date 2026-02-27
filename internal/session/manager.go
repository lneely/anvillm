// Package session manages backend sessions
package session

import (
	"anvillm/internal/backend"
	"anvillm/internal/backend/tmux"
	"anvillm/internal/eventbus"
	"anvillm/internal/logging"
	"anvillm/internal/mailbox"
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Manager holds all active sessions
type Manager struct {
	backends      map[string]backend.Backend
	sessions      map[string]backend.Session
	mailManager   *mailbox.Manager
	eventBus      *eventbus.Bus
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
		eventBus: nil, // Set via SetEventBus
		stopCh:      make(chan struct{}),
	}

	// Set session getter for alias lookup
	mailMgr.SetSessionGetter(m)

	// Set state change callback
	m.OnStateChange = func(sessionID, oldState, newState string) {
		if m.eventBus != nil {
			m.eventBus.Publish(sessionID, eventbus.EventStateChange, map[string]string{
				"old_state": oldState,
				"new_state": newState,
			})
		}
	}

	// Wire up mailbox event callbacks
	mailMgr.SetEventCallbacks(
		func(senderID string, msg *mailbox.Message) {
			if m.eventBus != nil {
				evType := eventbus.EventBotSend
				if senderID == "user" {
					evType = eventbus.EventUserSend
				}
				m.eventBus.Publish(senderID, evType, msg)
			}
		},
		func(receiverID string, msg *mailbox.Message) {
			if m.eventBus != nil {
				evType := eventbus.EventBotRecv
				if receiverID == "user" {
					evType = eventbus.EventUserRecv
				}
				m.eventBus.Publish(receiverID, evType, msg)
			}
		},
	)
	
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
		logging.Logger().Warn("backend not found", zap.String("backend", backendName))
		return nil, backend.ErrBackendNotFound
	}

	logging.Logger().Info("creating new session", zap.String("backend", backendName), zap.String("cwd", opts.CWD))
	sess, err := b.CreateSession(context.Background(), opts)
	if err != nil {
		logging.Logger().Error("failed to create session", zap.String("backend", backendName), zap.Error(err))
		return nil, err
	}

	// Wire up state change callback
	if tmuxSess, ok := sess.(*tmux.Session); ok {
		tmuxSess.OnStateChange = m.OnStateChange
		// Emit initial state change event
		if m.OnStateChange != nil {
			m.OnStateChange(sess.ID(), "stopped", sess.State())
		}
	}

	m.mu.Lock()
	m.sessions[sess.ID()] = sess
	m.mu.Unlock()

	// Create mailbox structure for new session
	m.mailManager.EnsureMailbox(sess.ID())

	logging.Logger().Info("session created", zap.String("id", sess.ID()), zap.String("backend", backendName))
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

// List returns all session IDs, sorted by creation time (newest first)
func (m *Manager) List() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	type entry struct {
		id      string
		created time.Time
	}

	entries := make([]entry, 0, len(m.sessions))
	for id, sess := range m.sessions {
		entries = append(entries, entry{id, sess.CreatedAt()})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].created.After(entries[j].created) // newest first
	})

	ids := make([]string, len(entries))
	for i, e := range entries {
		ids[i] = e.id
	}
	return ids
}

// Remove removes a session from the manager
func (m *Manager) Remove(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if _, exists := m.sessions[id]; exists {
		logging.Logger().Info("removing session", zap.String("id", id))
		delete(m.sessions, id)
	}
}

// Stop stops the mail processing loop
func (m *Manager) Stop() {
	close(m.stopCh)
	m.wg.Wait()
}

// mailProcessingLoop processes mailboxes every 5 seconds
func (m *Manager) mailProcessingLoop() {
	defer m.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			logging.Logger().Error("panic in mailProcessingLoop", zap.Any("panic", r))
		}
	}()
	
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
	defer func() {
		if r := recover(); r != nil {
			logging.Logger().Error("panic in processMailboxes", zap.Any("panic", r))
		}
	}()

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
				logging.Logger().Error("failed to peek outbox", zap.String("sender", senderID), zap.Error(err))
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
				logging.Logger().Warn("message undeliverable", zap.String("to", msg.To), zap.Error(err))
			} else {
				// Remove only after successful delivery
				m.mailManager.RemoveFromOutbox(senderID)
			}
		}
	}
	
	// 2. Process user inbox — all message types stay in inbox for user to review
	
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
		_, err := sess.Send(ctx, fmt.Sprintf("You have a new message. Read it using read_inbox (agent_id=%s) and respond appropriately.", sess.ID()))
		if err != nil {
			logging.Logger().Error("failed to prompt agent", zap.String("session", sess.ID()), zap.Error(err))
		}
	}
}

// GetMailManager returns the mailbox manager (guaranteed non-nil)
func (m *Manager) GetMailManager() *mailbox.Manager {
	return m.mailManager
}

// SetEventBus sets the event bus for emitting events.
func (m *Manager) SetEventBus(bus *eventbus.Bus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.eventBus = bus
}
