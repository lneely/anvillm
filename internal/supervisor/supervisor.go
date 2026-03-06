package supervisor

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	"anvillm/internal/eventbus"
	"anvillm/internal/mailbox"
	"anvillm/internal/p9"
	"anvillm/internal/session"
)

type Supervisor struct {
	sessions     *session.Manager
	beads        *p9.BeadsFS
	mailbox      *mailbox.Manager
	eventbus     *eventbus.Bus
	lastAssigned map[string]int64
	assigned     map[string]string // beadID -> botID
}

func New(s *session.Manager, b *p9.BeadsFS, m *mailbox.Manager, e *eventbus.Bus) *Supervisor {
	return &Supervisor{s, b, m, e, make(map[string]int64), make(map[string]string)}
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
	
	// Filter for claimable, unassigned tasks
	claimable := []map[string]interface{}{}
	for _, bead := range beads {
		// Skip if already assigned
		if assignee, ok := bead["assignee"].(string); ok && assignee != "" {
			continue
		}
		// Only consider beads labeled claimable
		labels, ok := bead["labels"].([]interface{})
		if !ok {
			continue
		}
		for _, label := range labels {
			if labelStr, ok := label.(string); ok && labelStr == "claimable" {
				claimable = append(claimable, bead)
				break
			}
		}
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
			mountName := filepath.Base(taskCwd)
			
			// Skip if already assigned
			if _, assigned := s.assigned[beadID]; assigned {
				continue
			}
			
			msg := mailbox.NewMessage("supervisor", botID, mailbox.MessageTypePromptRequest, "work", "Work on bead "+beadID+", mount="+mountName+".")
			s.mailbox.DeliverToInbox(botID, msg)
			s.lastAssigned[botID] = time.Now().Unix()
			s.assigned[beadID] = botID
			break
		}
	}
}

func (s *Supervisor) Run(ctx context.Context) {
	events, cancel := s.eventbus.Subscribe()
	defer cancel()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	syncTicker := time.NewTicker(60 * time.Second)
	defer syncTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-events:
			if event.Type == eventbus.EventStateChange {
				if data, ok := event.Data.(map[string]string); ok {
					if data["new_state"] == "running" {
						botID := event.Agent
						sess := s.sessions.Get(botID)
						if sess != nil {
							cwd := sess.Metadata().Cwd
							mountName := filepath.Base(cwd)
							s.beads.Mount(mountName, cwd)
						}
					}
				}
			}
		case <-ticker.C:
			s.assignWork()
		case <-syncTicker.C:
			// Sync all mounts - need to add accessor method
			// s.beads.SyncAll()
		}
	}
}
