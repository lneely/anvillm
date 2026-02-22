package p9

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	bd "github.com/steveyegge/beads"
)

// BeadsFS handles beads filesystem operations.
type BeadsFS struct {
	store       bd.Storage
	ctx         context.Context
	lastQuery   *bd.IssueFilter
	queryResult []*bd.Issue
}

// NewBeadsFS creates a beads filesystem handler from an existing store.
func NewBeadsFS(store bd.Storage, ctx context.Context) *BeadsFS {
	return &BeadsFS{store: store, ctx: ctx}
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
	case len(parts) == 1 && parts[0] == "stats":
		return b.readStats()
	case len(parts) == 1 && parts[0] == "blocked":
		return b.readBlocked()
	case len(parts) == 1 && parts[0] == "stale":
		return b.readStale()
	case len(parts) == 1 && parts[0] == "query":
		return b.readQuery()
	case len(parts) == 1 && parts[0] == "config":
		return b.readAllConfig()
	case len(parts) == 2 && parts[0] == "search":
		return b.readSearch(parts[1])
	case len(parts) == 2 && parts[0] == "by-ref":
		return b.readByExternalRef(parts[1])
	case len(parts) == 2 && parts[0] == "config":
		return b.readConfig(parts[1])
	case len(parts) == 2 && parts[0] == "batch":
		return b.readBatch(parts[1])
	case len(parts) == 2 && parts[0] == "label":
		return b.readByLabel(parts[1])
	case len(parts) == 2 && parts[0] == "children":
		return b.readChildren(parts[1])
	case len(parts) == 2:
		return b.readBeadProperty(parts[0], parts[1])
	case len(parts) == 3 && parts[2] == "dependencies-meta":
		return b.readDependenciesMeta(parts[1])
	case len(parts) == 3 && parts[2] == "dependents-meta":
		return b.readDependentsMeta(parts[1])
	default:
		return nil, fmt.Errorf("invalid path: %s", path)
	}
}

// Write handles writes to beads filesystem.
func (b *BeadsFS) Write(path string, data []byte, sessionID string) error {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	
	if len(parts) == 1 && parts[0] == "query" {
		return b.executeQuery(data)
	}
	
	if len(parts) != 1 || parts[0] != "ctl" {
		return fmt.Errorf("write not allowed: %s", path)
	}
	
	return b.executeCtl(string(data))
}

func (b *BeadsFS) readList() ([]byte, error) {
	issues, err := b.store.SearchIssues(b.ctx, "", bd.IssueFilter{})
	if err != nil {
		return nil, err
	}
	// Sort by ID for hierarchical view (bd-a3f8, bd-a3f8.1, bd-a3f8.1.1, etc.)
	sort.Slice(issues, func(i, j int) bool {
		return issues[i].ID < issues[j].ID
	})

	// Enrich with blockers
	type BeadWithBlockers struct {
		*bd.Issue
		Blockers []string `json:"blockers,omitempty"`
	}
	result := make([]BeadWithBlockers, len(issues))
	for i, issue := range issues {
		result[i].Issue = issue
		if blockers, err := b.getBlockers(issue.ID); err == nil && len(blockers) > 0 {
			result[i].Blockers = blockers
		}
	}

	return json.MarshalIndent(result, "", "  ")
}

func (b *BeadsFS) readReady(role string) ([]byte, error) {
	filter := bd.WorkFilter{}
	issues, err := b.store.GetReadyWork(b.ctx, filter)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(issues, "", "  ")
}

func (b *BeadsFS) readSearch(query string) ([]byte, error) {
	issues, err := b.store.SearchIssues(b.ctx, query, bd.IssueFilter{})
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(issues, "", "  ")
}

func (b *BeadsFS) readByExternalRef(ref string) ([]byte, error) {
	issue, err := b.store.GetIssueByExternalRef(b.ctx, ref)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(issue, "", "  ")
}

func (b *BeadsFS) readConfig(key string) ([]byte, error) {
	value, err := b.store.GetConfig(b.ctx, key)
	if err != nil {
		return nil, err
	}
	return []byte(value), nil
}

func (b *BeadsFS) readAllConfig() ([]byte, error) {
	config, err := b.store.GetAllConfig(b.ctx)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(config, "", "  ")
}

func (b *BeadsFS) readBatch(ids string) ([]byte, error) {
	idList := strings.Split(ids, ",")
	issues, err := b.store.GetIssuesByIDs(b.ctx, idList)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(issues, "", "  ")
}

func (b *BeadsFS) readByLabel(label string) ([]byte, error) {
	issues, err := b.store.GetIssuesByLabel(b.ctx, label)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(issues, "", "  ")
}

func (b *BeadsFS) readBlocked() ([]byte, error) {
	filter := bd.WorkFilter{}
	blocked, err := b.store.GetBlockedIssues(b.ctx, filter)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(blocked, "", "  ")
}

func (b *BeadsFS) readDependenciesMeta(beadID string) ([]byte, error) {
	deps, err := b.store.GetDependenciesWithMetadata(b.ctx, beadID)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(deps, "", "  ")
}

func (b *BeadsFS) readDependentsMeta(beadID string) ([]byte, error) {
	deps, err := b.store.GetDependentsWithMetadata(b.ctx, beadID)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(deps, "", "  ")
}

func (b *BeadsFS) readChildren(parentID string) ([]byte, error) {
	filter := bd.IssueFilter{ParentID: &parentID}
	children, err := b.store.SearchIssues(b.ctx, "", filter)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(children, "", "  ")
}

func (b *BeadsFS) readStale() ([]byte, error) {
	// Get all non-closed issues
	filter := bd.IssueFilter{}
	issues, err := b.store.SearchIssues(b.ctx, "", filter)
	if err != nil {
		return nil, err
	}
	
	// Filter to stale issues (not updated in 30+ days, status open or in_progress)
	cutoff := time.Now().AddDate(0, 0, -30)
	var stale []*bd.Issue
	for _, issue := range issues {
		if issue.Status != bd.StatusClosed && issue.UpdatedAt.Before(cutoff) {
			stale = append(stale, issue)
		}
	}
	
	return json.MarshalIndent(stale, "", "  ")
}

func (b *BeadsFS) executeQuery(data []byte) error {
	var filter bd.IssueFilter
	if err := json.Unmarshal(data, &filter); err != nil {
		return fmt.Errorf("invalid JSON filter: %w", err)
	}
	
	issues, err := b.store.SearchIssues(b.ctx, "", filter)
	if err != nil {
		return err
	}
	
	b.lastQuery = &filter
	b.queryResult = issues
	return nil
}

func (b *BeadsFS) readQuery() ([]byte, error) {
	if b.queryResult == nil {
		return json.MarshalIndent([]*bd.Issue{}, "", "  ")
	}
	return json.MarshalIndent(b.queryResult, "", "  ")
}

func (b *BeadsFS) readStats() ([]byte, error) {
	stats, err := b.store.GetStatistics(b.ctx)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(stats, "", "  ")
}

func (b *BeadsFS) readBeadProperty(beadID, property string) ([]byte, error) {
	issue, err := b.store.GetIssue(b.ctx, beadID)
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
			*bd.Issue
			Blockers []string `json:"blockers,omitempty"`
		}
		result := BeadWithBlockers{Issue: issue}
		if blockers, err := b.getBlockers(beadID); err == nil {
			result.Blockers = blockers
		}
		return json.MarshalIndent(result, "", "  ")
	case "comments":
		comments, err := b.store.GetIssueComments(b.ctx, beadID)
		if err != nil {
			return nil, err
		}
		return json.MarshalIndent(comments, "", "  ")
	case "labels":
		labels, err := b.store.GetLabels(b.ctx, beadID)
		if err != nil {
			return nil, err
		}
		return json.MarshalIndent(labels, "", "  ")
	case "dependents":
		dependents, err := b.store.GetDependents(b.ctx, beadID)
		if err != nil {
			return nil, err
		}
		return json.MarshalIndent(dependents, "", "  ")
	case "tree":
		tree, err := b.store.GetDependencyTree(b.ctx, beadID, 10, false, false)
		if err != nil {
			return nil, err
		}
		return json.MarshalIndent(tree, "", "  ")
	case "events":
		events, err := b.store.GetEvents(b.ctx, beadID, 100)
		if err != nil {
			return nil, err
		}
		return json.MarshalIndent(events, "", "  ")
	default:
		return nil, fmt.Errorf("unknown property: %s", property)
	}
}

func (b *BeadsFS) getBlockers(id string) ([]string, error) {
	deps, err := b.store.GetDependenciesWithMetadata(b.ctx, id)
	if err != nil {
		return nil, err
	}
	var blockers []string
	for _, dep := range deps {
		if dep.DependencyType == bd.DepBlocks && dep.Status != bd.StatusClosed {
			blockers = append(blockers, dep.ID)
		}
	}
	return blockers, nil
}

func (b *BeadsFS) executeCtl(cmd string) error {
	command, args, err := parseCtlCommand(cmd)
	if err != nil {
		return err
	}
	
	actor := "user"
	
	switch command {
	case "init":
		prefix := "bd"
		if len(args) > 0 {
			prefix = args[0]
		}
		return b.store.SetConfig(b.ctx, "issue_prefix", prefix)
		
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
			return b.createSubtask(parentID, title, description, actor)
		}
		
		issue := &bd.Issue{
			Title:       title,
			Description: description,
			Status:      bd.StatusOpen,
			IssueType:   bd.TypeTask,
			Priority:    2,
		}
		return b.store.CreateIssue(b.ctx, issue, actor)
		
	case "claim":
		if len(args) < 1 {
			return fmt.Errorf("usage: claim <bead-id> [assignee]")
		}
		assignee := "user"
		if len(args) > 1 {
			assignee = args[1]
		}
		updates := map[string]interface{}{
			"assignee": assignee,
			"status":   bd.StatusInProgress,
		}
		return b.store.UpdateIssue(b.ctx, args[0], updates, actor)
		
	case "complete", "close":
		if len(args) < 1 {
			return fmt.Errorf("usage: complete <bead-id>")
		}
		return b.store.CloseIssue(b.ctx, args[0], "completed", actor, "")
		
	case "fail":
		if len(args) < 2 {
			return fmt.Errorf("usage: fail <bead-id> 'reason'")
		}
		return b.store.CloseIssue(b.ctx, args[0], args[1], actor, "")
		
	case "dep", "add-dep":
		if len(args) < 2 {
			return fmt.Errorf("usage: dep <child-id> <parent-id>")
		}
		dep := &bd.Dependency{
			IssueID:     args[0],
			DependsOnID: args[1],
			Type:        bd.DepBlocks,
		}
		return b.store.AddDependency(b.ctx, dep, actor)

	case "undep", "rm-dep":
		if len(args) < 2 {
			return fmt.Errorf("usage: undep <child-id> <parent-id>")
		}
		return b.store.RemoveDependency(b.ctx, args[0], args[1], actor)

	case "update":
		if len(args) < 3 {
			return fmt.Errorf("usage: update <bead-id> <field> 'value'")
		}
		updates := map[string]interface{}{
			args[1]: args[2],
		}
		return b.store.UpdateIssue(b.ctx, args[0], updates, actor)

	case "delete", "rm":
		if len(args) < 1 {
			return fmt.Errorf("usage: delete <bead-id>")
		}
		return b.store.DeleteIssue(b.ctx, args[0])

	case "comment":
		if len(args) < 2 {
			return fmt.Errorf("usage: comment <bead-id> 'text'")
		}
		_, err := b.store.AddIssueComment(b.ctx, args[0], actor, args[1])
		return err

	case "label":
		if len(args) < 2 {
			return fmt.Errorf("usage: label <bead-id> 'label'")
		}
		return b.store.AddLabel(b.ctx, args[0], args[1], actor)

	case "unlabel":
		if len(args) < 2 {
			return fmt.Errorf("usage: unlabel <bead-id> 'label'")
		}
		return b.store.RemoveLabel(b.ctx, args[0], args[1], actor)

	case "batch-create":
		if len(args) < 1 {
			return fmt.Errorf("usage: batch-create <json-array>")
		}
		var issues []*bd.Issue
		if err := json.Unmarshal([]byte(args[0]), &issues); err != nil {
			return fmt.Errorf("invalid JSON: %w", err)
		}
		return b.store.CreateIssues(b.ctx, issues, actor)
		
	default:
		return fmt.Errorf("unknown command: %s (supported: init, new, update, delete, claim, complete, fail, dep, undep)", command)
	}
}

func (b *BeadsFS) createSubtask(parentID, title, description, actor string) error {
	// Verify parent exists
	parent, err := b.store.GetIssue(b.ctx, parentID)
	if err != nil {
		return fmt.Errorf("failed to get parent: %w", err)
	}
	if parent == nil {
		return fmt.Errorf("parent %s not found", parentID)
	}

	// Find next child number by scanning existing IDs
	nextChild := 1
	issues, err := b.store.SearchIssues(b.ctx, "", bd.IssueFilter{})
	if err == nil {
		prefix := parentID + "."
		for _, issue := range issues {
			if strings.HasPrefix(issue.ID, prefix) {
				suffix := strings.TrimPrefix(issue.ID, prefix)
				if !strings.Contains(suffix, ".") {
					if n, err := strconv.Atoi(suffix); err == nil && n >= nextChild {
						nextChild = n + 1
					}
				}
			}
		}
	}

	childID := fmt.Sprintf("%s.%d", parentID, nextChild)

	issue := &bd.Issue{
		ID:          childID,
		Title:       title,
		Description: description,
		Status:      bd.StatusOpen,
		IssueType:   bd.TypeTask,
		Priority:    2,
	}

	return b.store.CreateIssue(b.ctx, issue, actor)
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
