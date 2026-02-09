// Package session manages backend sessions
package session

import (
	"acme-q/internal/backend"
	"context"
	"sync"
)

// Manager holds all active sessions
type Manager struct {
	backend  backend.Backend
	sessions map[string]backend.Session
	mu       sync.RWMutex
}

// NewManager creates a session manager with the given backend
func NewManager(b backend.Backend) *Manager {
	m := &Manager{
		backend:  b,
	}
	m.sessions = make(map[string]backend.Session)
	return m
}

// New creates a new session in the given working directory
func (m *Manager) New(cwd string) (backend.Session, error) {
	sess, err := m.backend.CreateSession(context.Background(), cwd)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	m.sessions[sess.ID()] = sess
	m.mu.Unlock()

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
