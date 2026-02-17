package mailbox

import (
	"anvillm/internal/audit"
	"fmt"
	"sync"
	"time"
)

// SessionGetter provides access to session aliases
type SessionGetter interface {
	GetAlias(id string) string
}

// Manager handles mailbox operations for all sessions (in-memory)
type Manager struct {
	// sessionID -> list of messages
	inboxes   map[string][]*Message
	outboxes  map[string][]*Message
	completed map[string][]*Message
	mu        sync.RWMutex
	idCounter uint64
	auditLog  *audit.Log
	sessions  SessionGetter
}

// NewManager creates a new mailbox manager
func NewManager() *Manager {
	m := &Manager{
		inboxes:   make(map[string][]*Message),
		outboxes:  make(map[string][]*Message),
		completed: make(map[string][]*Message),
		auditLog:  audit.NewLog(),
	}
	// Initialize user mailbox
	m.inboxes["user"] = []*Message{}
	m.outboxes["user"] = []*Message{}
	m.completed["user"] = []*Message{}
	return m
}

// SetSessionGetter sets the session getter for alias lookup
func (m *Manager) SetSessionGetter(sg SessionGetter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions = sg
}

// EnsureMailbox initializes mailbox for a session (no-op for in-memory)
func (m *Manager) EnsureMailbox(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if _, ok := m.inboxes[sessionID]; !ok {
		m.inboxes[sessionID] = []*Message{}
		m.outboxes[sessionID] = []*Message{}
		m.completed[sessionID] = []*Message{}
	}
	
	return nil
}

// AddToOutbox adds a message to a session's outbox
func (m *Manager) AddToOutbox(sessionID string, msg *Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// Set from field if empty
	if msg.From == "" {
		msg.From = sessionID
	}
	
	// Generate ID if empty
	if msg.ID == "" {
		msg.ID = generateID()
	}
	
	// Set timestamp if zero
	if msg.Timestamp == 0 {
		msg.Timestamp = time.Now().Unix()
	}
	
	m.outboxes[sessionID] = append(m.outboxes[sessionID], msg)
	return nil
}

// HasOutbox checks if a session has messages in outbox
func (m *Manager) HasOutbox(sessionID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	return len(m.outboxes[sessionID]) > 0
}

// PeekOutbox reads the first message from outbox without removing it
func (m *Manager) PeekOutbox(sessionID string) (*Message, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	msgs := m.outboxes[sessionID]
	if len(msgs) == 0 {
		return nil, fmt.Errorf("no messages in outbox")
	}
	
	return msgs[0], nil
}

// RemoveFromOutbox removes the first message from outbox
func (m *Manager) RemoveFromOutbox(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	msgs := m.outboxes[sessionID]
	if len(msgs) == 0 {
		return fmt.Errorf("no messages in outbox")
	}
	
	m.outboxes[sessionID] = msgs[1:]
	return nil
}

// DeliverToInbox delivers a message to a session's inbox
func (m *Manager) DeliverToInbox(sessionID string, msg *Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// Check if receiver exists
	if _, exists := m.inboxes[sessionID]; !exists {
		return fmt.Errorf("receiver %s does not exist", sessionID)
	}
	
	m.inboxes[sessionID] = append(m.inboxes[sessionID], msg)
	
	// Format sender and receiver with aliases
	sender := m.formatParticipant(msg.From)
	receiver := m.formatParticipant(msg.To)
	
	// Audit log the delivery
	m.auditLog.Append(string(msg.Type), sender, receiver, msg.Subject, msg.Body)
	
	return nil
}

// formatParticipant formats a participant ID with alias if available
// Returns "session-id" if no alias, or "alias(session-id)" if alias is set
func (m *Manager) formatParticipant(id string) string {
	if m.sessions == nil {
		return id
	}
	
	alias := m.sessions.GetAlias(id)
	if alias == "" {
		return id
	}
	
	return fmt.Sprintf("%s(%s)", alias, id)
}

// GetInbox returns all messages in inbox (copy to prevent modification)
func (m *Manager) GetInbox(sessionID string) []*Message {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	msgs := m.inboxes[sessionID]
	result := make([]*Message, len(msgs))
	copy(result, msgs)
	return result
}

// GetOutbox returns all messages in outbox (copy to prevent modification)
func (m *Manager) GetOutbox(sessionID string) []*Message {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	msgs := m.outboxes[sessionID]
	result := make([]*Message, len(msgs))
	copy(result, msgs)
	return result
}

// GetCompleted returns all completed messages (copy to prevent modification)
func (m *Manager) GetCompleted(sessionID string) []*Message {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	msgs := m.completed[sessionID]
	result := make([]*Message, len(msgs))
	copy(result, msgs)
	return result
}

// GetPendingMessages returns all pending messages in inbox
func (m *Manager) GetPendingMessages(sessionID string) ([]*Message, error) {
	return m.GetInbox(sessionID), nil
}

// PullMessage retrieves and removes the first message from inbox
func (m *Manager) PullMessage(sessionID string) (*Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	msgs := m.inboxes[sessionID]
	if len(msgs) == 0 {
		return nil, fmt.Errorf("no messages in inbox")
	}
	
	msg := msgs[0]
	m.inboxes[sessionID] = msgs[1:]
	return msg, nil
}

// PeekInbox returns first message without removing it
func (m *Manager) PeekInbox(sessionID string) (*Message, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	msgs := m.inboxes[sessionID]
	if len(msgs) == 0 {
		return nil, fmt.Errorf("no messages in inbox")
	}
	
	return msgs[0], nil
}

// HasPendingMessages checks if inbox has messages
func (m *Manager) HasPendingMessages(sessionID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	return len(m.inboxes[sessionID]) > 0
}

// CompleteMessage moves a message from inbox to completed
func (m *Manager) CompleteMessage(sessionID, msgID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// Find and remove from inbox
	inbox := m.inboxes[sessionID]
	for i, msg := range inbox {
		if msg.ID == msgID {
			// Remove from inbox
			m.inboxes[sessionID] = append(inbox[:i], inbox[i+1:]...)
			// Add to completed
			m.completed[sessionID] = append(m.completed[sessionID], msg)
			return nil
		}
	}
	
	return fmt.Errorf("message not found in inbox")
}

// GetMessage retrieves a specific message by ID from any mailbox
func (m *Manager) GetMessage(sessionID, msgID string) (*Message, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	// Check inbox
	for _, msg := range m.inboxes[sessionID] {
		if msg.ID == msgID {
			return msg, nil
		}
	}
	
	// Check outbox
	for _, msg := range m.outboxes[sessionID] {
		if msg.ID == msgID {
			return msg, nil
		}
	}
	
	// Check completed
	for _, msg := range m.completed[sessionID] {
		if msg.ID == msgID {
			return msg, nil
		}
	}
	
	return nil, fmt.Errorf("message not found")
}

// MoveToCompleted moves a message to the completed folder
func (m *Manager) MoveToCompleted(sessionID string, msg *Message) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.completed[sessionID] = append(m.completed[sessionID], msg)
}

// GetAuditLog returns the audit log
func (m *Manager) GetAuditLog() *audit.Log {
	return m.auditLog
}
