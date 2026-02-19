// Package events implements a simple event queue with TTL and explicit ack.
package events

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"
)

const DefaultTTL = 2 * time.Minute

type Event struct {
	ID     string `json:"id"`
	TS     int64  `json:"ts"`
	Agent  string `json:"agent"`
	Type   string `json:"type"`
	Data   any    `json:"data"`
	expiry time.Time
}

type Queue struct {
	mu     sync.Mutex
	events []*Event
	ttl    time.Duration
}

func NewQueue() *Queue {
	q := &Queue{ttl: DefaultTTL}
	go q.expireLoop()
	return q
}

// Push adds an event to the queue.
func (q *Queue) Push(agent, eventType string, data any) string {
	q.mu.Lock()
	defer q.mu.Unlock()

	e := &Event{
		ID:     uuid.New().String(),
		TS:     time.Now().Unix(),
		Agent:  agent,
		Type:   eventType,
		Data:   data,
		expiry: time.Now().Add(q.ttl),
	}
	q.events = append(q.events, e)
	return e.ID
}

// Read returns all pending events as JSON lines.
func (q *Queue) Read() []byte {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.events) == 0 {
		return nil
	}

	var out []byte
	for _, e := range q.events {
		line, _ := json.Marshal(e)
		out = append(out, line...)
		out = append(out, '\n')
	}
	return out
}

// AckRequest is the JSON payload for acknowledging events.
type AckRequest struct {
	Msg string   `json:"msg"`
	IDs []string `json:"ids"`
}

// Ack removes events with the given IDs.
func (q *Queue) Ack(data []byte) error {
	var req AckRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return err
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	idSet := make(map[string]bool, len(req.IDs))
	for _, id := range req.IDs {
		idSet[id] = true
	}

	filtered := q.events[:0]
	for _, e := range q.events {
		if !idSet[e.ID] {
			filtered = append(filtered, e)
		}
	}
	q.events = filtered
	return nil
}

func (q *Queue) expireLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		q.expire()
	}
}

func (q *Queue) expire() {
	q.mu.Lock()
	defer q.mu.Unlock()

	now := time.Now()
	filtered := q.events[:0]
	for _, e := range q.events {
		if e.expiry.After(now) {
			filtered = append(filtered, e)
		}
	}
	q.events = filtered
}
