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
	
	// 1. Deliver outbound messages (from sessions and user)
	allSenders := append([]string{"user"}, m.List()...)
	for _, senderID := range allSenders {
		if m.mailManager.HasOutbox(senderID) {
			msg, err := m.mailManager.ReadOutbox(senderID)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reading outbox for %s: %v\n", senderID, err)
				continue
			}
			
			// Deliver to recipient
			if err := m.mailManager.DeliverToInbox(msg.To, msg); err != nil {
				fmt.Fprintf(os.Stderr, "Error delivering message to %s: %v\n", msg.To, err)
			}
		}
	}
	
	// 2. Process user inbox - write to sender's log
	userMessages, _ := m.mailManager.GetPendingMessages("user")
	for _, msg := range userMessages {
		// Move to completed
		if err := m.mailManager.CompleteMessage("user", msg.ID); err != nil {
			continue
		}
		
		// Write human-readable output to sender's log
		if senderSess := m.Get(msg.From); senderSess != nil {
			if tmuxSess, ok := senderSess.(*tmux.Session); ok {
				tmuxSess.AppendToChatLog("ASSISTANT", msg.Body)
			}
		}
	}
	
	// 3. Process inbound messages for idle sessions
	for _, sess := range sessions {
		if sess.State() == "idle" {
			messages, err := m.mailManager.GetPendingMessages(sess.ID())
			if err != nil || len(messages) == 0 {
				continue
			}
			
			// Process first message
			msg := messages[0]
			
			// Move to completed BEFORE sending to prevent duplicate processing
			if err := m.mailManager.CompleteMessage(sess.ID(), msg.ID); err != nil {
				continue
			}
			
			// Format message for bot
			prompt := fmt.Sprintf("[Message from %s]\nType: %s\nSubject: %s\n\n%s", 
				msg.From, msg.Type, msg.Subject, msg.Body)
			
			// Send to bot (non-blocking)
			go func(s backend.Session, p string, message *mailbox.Message) {
				_, err := s.Send(context.Background(), p)
				if err != nil {
					message.Retries++
					if message.Retries > 3 {
						// Discard message after 3 failed attempts
						fmt.Fprintf(os.Stderr, "Message %s from %s to %s failed after 3 retries, discarding\nSubject: %s\nBody: %s\n", 
							message.ID, message.From, s.ID(), message.Subject, message.Body)
						
						// Notify sender via their chat log (visible to human)
						if message.From != "" && message.From != "user" {
							if senderSess := m.Get(message.From); senderSess != nil {
								if tmuxSess, ok := senderSess.(*tmux.Session); ok {
									notification := fmt.Sprintf("\n[DELIVERY FAILURE]\nFailed to deliver message to %s after 3 attempts.\nSubject: %s\nBody: %s\n\n", 
										s.ID(), message.Subject, message.Body)
									tmuxSess.AppendToChatLog("SYSTEM", notification)
								}
							}
						}
					} else {
						fmt.Fprintf(os.Stderr, "Error sending message to %s (retry %d/3): %v\n", s.ID(), message.Retries, err)
						// Restore message to inbox for retry
						m.mailManager.DeliverToInbox(s.ID(), message)
					}
				}
			}(sess, prompt, msg)
		}
	}
}

// GetMailManager returns the mailbox manager
func (m *Manager) GetMailManager() *mailbox.Manager {
	return m.mailManager
}
