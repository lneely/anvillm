// Package maildir persists messages to append-only JSONL files.
package maildir

import (
	"anvillm/internal/eventbus"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Writer subscribes to the event bus and persists messages to JSONL files.
type Writer struct {
	baseDir string
	mu      sync.Mutex
	cancel  func()
}

// New creates a Writer that persists messages under baseDir.
func New(baseDir string, bus *eventbus.Bus) *Writer {
	w := &Writer{baseDir: baseDir}
	ch, cancel := bus.Subscribe()
	w.cancel = cancel
	go w.run(ch)
	return w
}

func (w *Writer) run(ch <-chan *eventbus.Event) {
	for ev := range ch {
		var suffix string
		switch ev.Type {
		case eventbus.EventUserRecv, eventbus.EventBotRecv:
			suffix = "recv"
		case eventbus.EventUserSend, eventbus.EventBotSend:
			suffix = "sent"
		default:
			continue
		}
		w.append(ev.Source, suffix, ev)
	}
}

func (w *Writer) append(agent, suffix string, ev *eventbus.Event) {
	w.mu.Lock()
	defer w.mu.Unlock()

	dir := filepath.Join(w.baseDir, agent)
	os.MkdirAll(dir, 0755)

	date := time.Unix(ev.TS, 0).Format("20060102")
	path := filepath.Join(dir, date+"-"+suffix+".jsonl")

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	data, _ := json.Marshal(ev)
	f.Write(append(data, '\n'))
}

// Close stops the writer.
func (w *Writer) Close() {
	if w.cancel != nil {
		w.cancel()
	}
}
