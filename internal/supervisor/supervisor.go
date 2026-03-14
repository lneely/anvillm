package supervisor

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"anvillm/internal/eventbus"
	"anvillm/internal/mailbox"
	"anvillm/internal/p9"
	"anvillm/internal/session"
)

type Supervisor struct {
	sessions *session.Manager
	beads    *p9.BeadsFS
	mailbox  *mailbox.Manager
	eventbus *eventbus.Bus
}

func New(s *session.Manager, b *p9.BeadsFS, m *mailbox.Manager, e *eventbus.Bus) *Supervisor {
	return &Supervisor{s, b, m, e}
}

func (s *Supervisor) assignWork() {
	// Skip if beads not available
	if s.beads == nil {
		return
	}
	
	// Get all ready tasks from all projects
	ready, err := s.beads.Read("ready")
	if err != nil || len(ready) == 0 {
		return
	}
	
	var beads []map[string]interface{}
	if err := json.Unmarshal(ready, &beads); err != nil {
		return
	}
	
	// Filter for unassigned tasks
	claimable := []map[string]interface{}{}
	for _, bead := range beads {
		if assignee, ok := bead["assignee"].(string); ok && assignee != "" {
			continue
		}
		claimable = append(claimable, bead)
	}

	if len(claimable) == 0 {
		return
	}
	
	// Try to assign to idle bots with matching role
	bots := s.sessions.List()
	for _, botID := range bots {
		sess := s.sessions.Get(botID)
		state := sess.State()
		if state != "idle" {
			continue
		}
		botCwd := sess.Metadata().Cwd
		
		// Get bot role (default to "developer" if not set)
		botRole := "developer"
		if tmuxSess, ok := sess.(interface{ GetRole() string }); ok {
			if role := tmuxSess.GetRole(); role != "" {
				botRole = role
			}
		}
		
		// Only assign if session CWD is exactly or under task mountpoint
		for _, bead := range claimable {
			taskCwd, ok := bead["cwd"].(string)
			if !ok {
				continue
			}
			
			if botCwd != taskCwd && !strings.HasPrefix(botCwd, taskCwd+"/") {
				continue
			}
			
			// Extract role label from bead (format: "role:developer")
			beadRole := "developer" // default
			if labels, ok := bead["labels"].([]interface{}); ok {
				for _, label := range labels {
					if labelStr, ok := label.(string); ok {
						if role, found := strings.CutPrefix(labelStr, "role:"); found {
							beadRole = role
							break
						}
					}
				}
			}
			
			// Skip if role doesn't match
			if beadRole != botRole {
				continue
			}
			
			beadID := bead["id"].(string)
			mountName, _ := bead["mount"].(string)

			msg := mailbox.NewMessage("supervisor", botID, mailbox.MessageTypePromptRequest, "work", "Work on bead "+beadID+", mount="+mountName+".")
			s.mailbox.DeliverToInbox(botID, msg)
			break
		}
	}
}

func (s *Supervisor) Run(ctx context.Context) {
	events, cancel := s.eventbus.Subscribe()
	defer cancel()
	// Fallback ticker: catches anything missed by events (e.g. startup, edge cases).
	fallback := time.NewTicker(60 * time.Second)
	defer fallback.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-events:
			if event == nil {
				continue
			}
			switch event.Type {
			case eventbus.EventStateChange:
				// Session went idle — it may be able to take work.
				if data, ok := event.Data.(map[string]string); ok && data["new_state"] == "idle" {
					s.assignWork()
				}
			case eventbus.EventBeadReady:
				// A bead just became open/ready — try to assign it.
				s.assignWork()
			}
		case <-fallback.C:
			s.assignWork()
		}
	}
}
