package p9

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"anvillm/internal/beads"
)

// BeadsFS handles beads filesystem operations.
type BeadsFS struct {
	store *beads.Store
}

// NewBeadsFS creates a beads filesystem handler from an existing store.
func NewBeadsFS(store *beads.Store) *BeadsFS {
	return &BeadsFS{store: store}
}

// Close is a no-op since the store is managed externally.
func (b *BeadsFS) Close() error {
	return nil
}

// Read handles reads from beads filesystem.
func (b *BeadsFS) Read(path string) ([]byte, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	
	switch {
	case len(parts) == 1 && parts[0] == "list":
		return b.readList()
	case len(parts) == 1 && parts[0] == "ready":
		return b.readReady("")
	case len(parts) == 2:
		return b.readBeadProperty(parts[0], parts[1])
	default:
		return nil, fmt.Errorf("invalid path: %s", path)
	}
}

// Write handles writes to beads filesystem.
func (b *BeadsFS) Write(path string, data []byte, sessionID string) error {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	
	if len(parts) != 1 || parts[0] != "ctl" {
		return fmt.Errorf("write not allowed: %s", path)
	}
	
	return b.executeCtl(string(data), sessionID)
}

func (b *BeadsFS) readList() ([]byte, error) {
	issues, err := b.store.ListBeads(beads.IssueFilter{})
	if err != nil {
		return nil, err
	}
	// Sort by ID for hierarchical view (bd-a3f8, bd-a3f8.1, bd-a3f8.1.1, etc.)
	sort.Slice(issues, func(i, j int) bool {
		return issues[i].ID < issues[j].ID
	})

	// Enrich with blockers
	type BeadWithBlockers struct {
		*beads.Issue
		Blockers []string `json:"blockers,omitempty"`
	}
	result := make([]BeadWithBlockers, len(issues))
	for i, issue := range issues {
		result[i].Issue = issue
		if blockers, err := b.store.GetBlockers(issue.ID); err == nil && len(blockers) > 0 {
			result[i].Blockers = blockers
		}
	}

	return json.MarshalIndent(result, "", "  ")
}

func (b *BeadsFS) readReady(role string) ([]byte, error) {
	issues, err := b.store.ReadyBeads(role)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(issues, "", "  ")
}

func (b *BeadsFS) readBeadProperty(beadID, property string) ([]byte, error) {
	issue, err := b.store.GetBead(beadID)
	if err != nil {
		return nil, err
	}
	
	switch property {
	case "status":
		return []byte(string(issue.Status)), nil
	case "title":
		return []byte(issue.Title), nil
	case "description":
		return []byte(issue.Description), nil
	case "assignee":
		return []byte(issue.Assignee), nil
	case "json":
		type BeadWithBlockers struct {
			*beads.Issue
			Blockers []string `json:"blockers,omitempty"`
		}
		result := BeadWithBlockers{Issue: issue}
		if blockers, err := b.store.GetBlockers(beadID); err == nil {
			result.Blockers = blockers
		}
		return json.MarshalIndent(result, "", "  ")
	default:
		return nil, fmt.Errorf("unknown property: %s", property)
	}
}

func (b *BeadsFS) executeCtl(cmd string, sessionID string) error {
	command, args, err := parseCtlCommand(cmd)
	if err != nil {
		return err
	}
	
	actor := sessionID
	if actor == "" {
		actor = "system"
	}
	
	switch command {
	case "init":
		prefix := "bd"
		if len(args) > 0 {
			prefix = args[0]
		}
		return b.store.Init(prefix)
		
	case "new", "create":
		if len(args) < 1 {
			return fmt.Errorf("usage: new 'title' ['description'] [parent-id]")
		}
		title := args[0]
		description := ""
		parentID := ""
		if len(args) > 1 {
			description = args[1]
		}
		if len(args) > 2 {
			parentID = args[2]
		}
		if parentID != "" {
			_, err = b.store.CreateSubtask(parentID, title, description, actor)
		} else {
			_, err = b.store.CreateBead(title, "", description, actor)
		}
		return err
		
	case "claim":
		if len(args) < 1 {
			return fmt.Errorf("usage: claim <bead-id>")
		}
		return b.store.ClaimBead(args[0], actor)
		
	case "complete", "close":
		if len(args) < 1 {
			return fmt.Errorf("usage: complete <bead-id>")
		}
		return b.store.CompleteBead(args[0], actor)
		
	case "fail":
		if len(args) < 2 {
			return fmt.Errorf("usage: fail <bead-id> 'reason'")
		}
		return b.store.FailBead(args[0], args[1], actor)
		
	case "dep", "add-dep":
		if len(args) < 2 {
			return fmt.Errorf("usage: dep <child-id> <parent-id>")
		}
		return b.store.AddDependency(args[0], args[1], actor)

	case "undep", "rm-dep":
		if len(args) < 2 {
			return fmt.Errorf("usage: undep <child-id> <parent-id>")
		}
		return b.store.RemoveDependency(args[0], args[1], actor)

	case "update":
		if len(args) < 3 {
			return fmt.Errorf("usage: update <bead-id> <field> 'value'")
		}
		return b.store.UpdateBead(args[0], args[1], args[2], actor)

	case "delete", "rm":
		if len(args) < 1 {
			return fmt.Errorf("usage: delete <bead-id>")
		}
		return b.store.DeleteBead(args[0])
		
	default:
		return fmt.Errorf("unknown command: %s (supported: init, new, update, delete, claim, complete, fail, dep, undep)", command)
	}
}

func parseCtlCommand(cmd string) (string, []string, error) {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return "", nil, fmt.Errorf("empty command")
	}
	
	command := parts[0]
	args := parseQuotedArgs(strings.TrimPrefix(cmd, command))
	return command, args, nil
}

func parseQuotedArgs(s string) []string {
	var args []string
	var current strings.Builder
	var quoteChar rune
	var wasQuoted bool
	
	s = strings.TrimSpace(s)
	for i := 0; i < len(s); i++ {
		c := rune(s[i])
		
		if quoteChar != 0 {
			// Inside quotes
			if c == quoteChar {
				quoteChar = 0 // End quote
				wasQuoted = true
			} else {
				current.WriteByte(s[i])
			}
		} else {
			// Outside quotes
			switch c {
			case '\'', '"':
				quoteChar = c // Start quote
			case ' ', '\t':
				if current.Len() > 0 || wasQuoted {
					args = append(args, current.String())
					current.Reset()
					wasQuoted = false
				}
			default:
				current.WriteByte(s[i])
			}
		}
	}
	
	if current.Len() > 0 || wasQuoted {
		args = append(args, current.String())
	}
	
	return args
}
