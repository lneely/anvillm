// Package eventbus implements an in-memory pub-sub event bus for anvillm events.
// It wraps github.com/simonfxr/pubsub to provide typed event publishing and
// per-subscriber streaming channels.
package eventbus

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	ps "github.com/simonfxr/pubsub"
)

// Event type constants.
const (
	EventStateChange = "StateChange"
	EventUserRecv    = "UserRecv"
	EventUserSend    = "UserSend"
	EventBotRecv     = "BotRecv"
	EventBotSend     = "BotSend"
)

// allTopic is the single topic used for all events.
const allTopic = "events"

// Event is the structure for all published events.
type Event struct {
	ID    string `json:"id"`
	TS    int64  `json:"ts"`
	Agent string `json:"agent"`
	Type  string `json:"type"`
	Data  any    `json:"data"`
}

// Bus is an in-memory pub-sub event bus.
// It is safe for concurrent use from multiple goroutines.
type Bus struct {
	bus *ps.Bus
}

// New creates a new Bus.
func New() *Bus {
	return &Bus{bus: ps.NewBus()}
}

// Publish emits an event to all current subscribers.
// It is non-blocking; slow subscribers will have events dropped.
func (b *Bus) Publish(agent, eventType string, data any) {
	e := &Event{
		ID:    uuid.New().String(),
		TS:    time.Now().Unix(),
		Agent: agent,
		Type:  eventType,
		Data:  data,
	}
	b.bus.Publish(allTopic, e)
}

// Subscribe returns a read channel that receives *Event values and a cancel
// function. The channel has a buffer of 64 events; events are dropped when
// the buffer is full (slow consumer). Calling cancel removes the subscription
// and closes the channel.
func (b *Bus) Subscribe() (<-chan *Event, func()) {
	ch := make(chan *Event, 64)
	sub := b.bus.SubscribeChan(allTopic, ch, ps.CloseOnUnsubscribe)
	cancel := func() {
		b.bus.Unsubscribe(sub)
	}
	return ch, cancel
}

// MarshalEvent encodes an event as a JSON line (with trailing newline).
func MarshalEvent(e *Event) []byte {
	data, _ := json.Marshal(e)
	return append(data, '\n')
}
