package p9

import (
	"anvillm/internal/logging"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	bd "github.com/steveyegge/beads"
	"go.uber.org/zap"
)

// Approval gate statuses extend the beads status set locally.
// These statuses are not surfaced by GetReadyWork (which only returns
// open/in_progress), ensuring bots do not claim work that is pending
// human approval or review.
const (
	StatusPendingApproval = bd.Status("pending_approval") // Awaiting APPROVAL_RESPONSE from human
	StatusPendingReview   = bd.Status("pending_review")   // Awaiting REVIEW_RESPONSE from human
)

// Capability level labels — portable model-tier hints carried on beads.
// These are label values of the form "capability:<level>".  The Conductor
// maps them to backend-specific model names at session-spawn time.
//
// Convention: use the minimum capable level.  When in doubt, prefer lower.
//   low      — haiku-tier: simple mechanical ops (create bead, update field,
//              send message, rename function, edit config)
//   standard — sonnet-tier: multi-file edits, moderate reasoning (default)
//   high     — opus-tier: novel design, ambiguous requirements, long-horizon
//              planning, concurrent algorithms, complex state machines
//
// Usage (add to a bead):
//
//	echo "label bd-abc capability:low" | 9p write agent/beads/ctl
//
// Usage (set at creation time):
//
//	echo "new 'title' 'desc' '' capability=low" | 9p write agent/beads/ctl
//
// Usage (read via JSON):
//
//	9p read agent/beads/bd-abc/json | jq -r .capability_level
//
// Refs: bd-frk.7, bd-frk.14.
const (
	CapabilityLow      = "capability:low"
	CapabilityStandard = "capability:standard"
	CapabilityHigh     = "capability:high"

	// capabilityPrefix is the label prefix used to identify capability labels.
	capabilityPrefix = "capability:"
)

// requiresApprovalLabel and requiresReviewLabel are label conventions for
// marking beads that must pass through a human gate before they can be closed.
// Agents should send an APPROVAL_REQUEST (or REVIEW_REQUEST) to the user and
// call `pending-approval <id>` (or `pending-review <id>`) before completing.
const (
	requiresApprovalLabel = "requires_approval"
	requiresReviewLabel   = "requires_review"
)

// MountedProject represents a project-specific beads instance.
type MountedProject struct {
	name       string
	cwd        string
	dbPath     string
	jsonlPath  string
	store      bd.Storage
}

// BeadsFS handles beads filesystem operations.
// BeadsFS provides 9P filesystem access to beads task tracking.
// It exposes beads operations through virtual files and directories.
type BeadsFS struct {
	ctx         context.Context
	lastQuery   *bd.IssueFilter
	queryResult []*bd.Issue
	mounts      map[string]*MountedProject
	mountsMu    sync.RWMutex
}

// NewBeadsFS creates a beads filesystem handler from an existing store.
func NewBeadsFS(store bd.Storage, ctx context.Context) *BeadsFS {
	return &BeadsFS{
		ctx:    ctx,
		mounts: make(map[string]*MountedProject),
	}
}

// Close is a no-op since the store is managed externally.
func (b *BeadsFS) Close() error {
	return nil
}

// ListMounts returns the names of all mounted projects.
func (b *BeadsFS) ListMounts() map[string]struct{} {
	b.mountsMu.RLock()
	defer b.mountsMu.RUnlock()
	result := make(map[string]struct{}, len(b.mounts))
	for name := range b.mounts {
		result[name] = struct{}{}
	}
	return result
}

// Mount adds a project-specific beads instance.
func (b *BeadsFS) Mount(name, cwd string) error {
	b.mountsMu.Lock()
	defer b.mountsMu.Unlock()
	if _, exists := b.mounts[name]; exists {
		return fmt.Errorf("mount %s exists", name)
	}
	// Check if cwd exists
	if _, err := os.Stat(cwd); os.IsNotExist(err) {
		return fmt.Errorf("directory does not exist: %s", cwd)
	} else if err != nil {
		return fmt.Errorf("failed to stat directory: %w", err)
	}
	// Use cwd as db path, replacing / with -
	cwdHyphenated := strings.ReplaceAll(cwd, "/", "-")
	dbPath := filepath.Join(os.Getenv("HOME"), ".beads", cwdHyphenated)
	if err := os.MkdirAll(dbPath, 0755); err != nil {
		return fmt.Errorf("failed to create db directory: %w", err)
	}
	jsonlPath := filepath.Join(cwd, ".beads", "issues.jsonl")
	store, err := bd.OpenFromConfig(b.ctx, dbPath)
	if err != nil {
		return err
	}
	// Import is not available in beads v0.54.0, skip for now
	b.mounts[name] = &MountedProject{name, cwd, dbPath, jsonlPath, store}
	return nil
}

// Umount removes a project-specific beads instance.
// Accepts either mount name or cwd path. If cwd matches multiple mounts, unmounts first match.
func (b *BeadsFS) Umount(nameOrCwd string) error {
	b.mountsMu.Lock()
	defer b.mountsMu.Unlock()
	
	// Try as mount name first
	m, ok := b.mounts[nameOrCwd]
	mountName := nameOrCwd
	if !ok {
		// Try as cwd - unmount first match
		for name, mount := range b.mounts {
			if mount.cwd == nameOrCwd {
				m = mount
				mountName = name
				ok = true
				break
			}
		}
	}
	
	if !ok {
		return fmt.Errorf("mount %s not found", nameOrCwd)
	}
	if err := m.store.Close(); err != nil {
		return fmt.Errorf("failed to close store: %w", err)
	}
	delete(b.mounts, mountName)
	return nil
}

// Sync exports a mounted project's beads to its jsonl file.
func (b *BeadsFS) Sync(name string) error {
	b.mountsMu.RLock()
	m := b.mounts[name]
	b.mountsMu.RUnlock()
	if m == nil {
		return fmt.Errorf("mount not found")
	}
	// Export is not available in beads v0.54.0, skip for now
	return nil
}


// Read handles reads from beads filesystem paths.
// Supports list, ready, pending, stats, blocked, stale, query, config endpoints,
// as well as search, batch, label, and per-bead property access.
func (b *BeadsFS) Read(path string) ([]byte, error) {
	// Check for mount paths (format: mountname/endpoint)
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) >= 2 {
		// Check if first part is a mount name
		b.mountsMu.RLock()
		m := b.mounts[parts[0]]
		b.mountsMu.RUnlock()
		if m != nil {
			endpoint := strings.Join(parts[1:], "/")
			if endpoint == "cwd" {
				return []byte(m.cwd), nil
			}
			// Route to mount-specific read
			return b.readFromMount(m, endpoint)
		}
	}
	
	if len(parts) == 0 {
		return nil, fmt.Errorf("invalid path: %s", path)
	}
	
	switch {
	case len(parts) == 1 && parts[0] == "mtab":
		return b.readMtab()
	case len(parts) == 1 && parts[0] == "ready":
		// If we have mounts, aggregate ready from all mounts
		b.mountsMu.RLock()
		hasMounts := len(b.mounts) > 0
		b.mountsMu.RUnlock()
		if hasMounts {
			return b.readReadyAggregate()
		}
		return nil, fmt.Errorf("no mounts")
	default:
		return nil, fmt.Errorf("invalid path: %s", path)
	}
}

func (b *BeadsFS) readFromMount(m *MountedProject, endpoint string) ([]byte, error) {
	parts := strings.Split(endpoint, "/")
	switch {
	case len(parts) == 1 && parts[0] == "list":
		return b.readListFromStore(m.store)
	case len(parts) == 1 && parts[0] == "ready":
		return b.readReadyFromStore(m.store)
	case len(parts) == 1 && parts[0] == "pending":
		return b.readPendingFromStore(m.store)
	case len(parts) == 1 && parts[0] == "blocked":
		return b.readBlockedFromStore(m.store)
	case len(parts) == 1 && parts[0] == "stale":
		return b.readStaleFromStore(m.store)
	case len(parts) == 2 && parts[0] == "search":
		return b.readSearchFromStore(m.store, parts[1])
	case len(parts) == 2 && parts[0] == "by-ref":
		return b.readByExternalRefFromStore(m.store, parts[1])
	case len(parts) == 2 && parts[0] == "batch":
		return b.readBatchFromStore(m.store, parts[1])
	case len(parts) == 2 && parts[0] == "label":
		return b.readByLabelFromStore(m.store, parts[1])
	case len(parts) == 2 && parts[0] == "children":
		return b.readChildrenFromStore(m.store, parts[1])
	case len(parts) == 2:
		return b.readBeadPropertyFromStore(m.store, parts[0], parts[1])
	case len(parts) == 3 && parts[2] == "dependencies-meta":
		return b.readDependenciesMetaFromStore(m.store, parts[1])
	case len(parts) == 3 && parts[2] == "dependents-meta":
		return b.readDependentsMetaFromStore(m.store, parts[1])
	default:
		return nil, fmt.Errorf("unsupported mount endpoint: %s", endpoint)
	}
}

// Write handles writes to beads filesystem paths.
// Supports query filter updates and ctl command execution.
func (b *BeadsFS) Write(path string, data []byte, sessionID string) error {
	// Check for mount paths (format: mountname/endpoint)
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) >= 2 {
		// Check if first part is a mount name
		b.mountsMu.RLock()
		m := b.mounts[parts[0]]
		b.mountsMu.RUnlock()
		if m != nil {
			logging.Logger().Info("mount write", zap.String("mount", parts[0]), zap.String("endpoint", strings.Join(parts[1:], "/")))
			endpoint := strings.Join(parts[1:], "/")
			return b.writeToMount(m, endpoint, data, sessionID)
		}
	}
	
	if len(parts) == 0 {
		return fmt.Errorf("invalid path: %s", path)
	}
	
	if len(parts) != 1 || parts[0] != "ctl" {
		return fmt.Errorf("write not allowed: %s", path)
	}
	
	// Global ctl commands (mount/umount/sync)
	command, args, err := parseCtlCommand(string(data))
	if err != nil {
		return err
	}
	
	switch command {
	case "mount":
		if len(args) < 1 {
			return fmt.Errorf("usage: mount <cwd> [name]")
		}
		cwd := args[0]
		name := filepath.Base(cwd)
		if len(args) >= 2 {
			name = args[1]
		}
		return b.Mount(name, cwd)
	case "umount":
		if len(args) < 1 {
			return fmt.Errorf("usage: umount <name>")
		}
		return b.Umount(args[0])
	case "sync":
		if len(args) < 1 {
			return fmt.Errorf("usage: sync <name>")
		}
		return b.Sync(args[0])
	default:
		return fmt.Errorf("command %s requires a mounted project", command)
	}
}

func (b *BeadsFS) writeToMount(m *MountedProject, endpoint string, data []byte, sessionID string) error {
	parts := strings.Split(endpoint, "/")
	if len(parts) == 1 && parts[0] == "ctl" {
		return b.executeCtlOnStore(m.store, string(data))
	}
	return fmt.Errorf("unsupported mount write endpoint: %s", endpoint)
}


func (b *BeadsFS) readMtab() ([]byte, error) {
	b.mountsMu.RLock()
	defer b.mountsMu.RUnlock()
	
	var lines []string
	for name, m := range b.mounts {
		lines = append(lines, name+"\t"+m.cwd)
	}
	return []byte(strings.Join(lines, "\n")), nil
}

func (b *BeadsFS) readListFromStore(store bd.Storage) ([]byte, error) {
	issues, err := store.SearchIssues(b.ctx, "", bd.IssueFilter{})
	if err != nil {
		return nil, err
	}
	if issues == nil {
		issues = []*bd.Issue{}
	}
	// Sort by ID for hierarchical view (bd-a3f8, bd-a3f8.1, bd-a3f8.1.1, etc.)
	sort.Slice(issues, func(i, j int) bool {
		return issues[i].ID < issues[j].ID
	})

	// Enrich with blockers, labels, and capability level.
	type BeadWithBlockers struct {
		*bd.Issue
		Blockers        []string `json:"blockers,omitempty"`
		CapabilityLevel string   `json:"capability_level,omitempty"`
	}
	result := make([]BeadWithBlockers, len(issues))
	for i, issue := range issues {
		// Fetch labels if not already populated
		if issue.Labels == nil {
			labels, err := store.GetLabels(b.ctx, issue.ID)
			if err == nil {
				issue.Labels = labels
			}
		}
		result[i].Issue = issue
		blockers, err := b.getBlockersFromStore(store, issue.ID)
		if err != nil {
			logging.Logger().Warn("failed to get blockers", zap.String("issue", issue.ID), zap.Error(err))
		} else if len(blockers) > 0 {
			result[i].Blockers = blockers
		}
		result[i].CapabilityLevel = extractCapabilityLevel(issue.Labels)
	}

	return json.MarshalIndent(result, "", "  ")
}


func (b *BeadsFS) readReadyFromStore(store bd.Storage) ([]byte, error) {
	filter := bd.WorkFilter{}
	issues, err := store.GetReadyWork(b.ctx, filter)
	if err != nil {
		return nil, err
	}
	if issues == nil {
		issues = []*bd.Issue{}
	}
	// Filter out blocked issues - ready means unblocked
	ready := []*bd.Issue{}
	for _, issue := range issues {
		// Fetch labels if not already populated
		if issue.Labels == nil {
			labels, err := store.GetLabels(b.ctx, issue.ID)
			if err == nil {
				issue.Labels = labels
			}
		}
		blockers, err := b.getBlockersFromStore(store, issue.ID)
		if err != nil || len(blockers) == 0 {
			ready = append(ready, issue)
		}
	}
	return json.MarshalIndent(ready, "", "  ")
}

func (b *BeadsFS) readReadyAggregate() ([]byte, error) {
	b.mountsMu.RLock()
	defer b.mountsMu.RUnlock()
	
	type TaskWithMount struct {
		*bd.Issue
		Mount string `json:"mount"`
		Cwd   string `json:"cwd"`
	}
	
	allReady := []TaskWithMount{}
	for name, m := range b.mounts {
		filter := bd.WorkFilter{}
		issues, err := m.store.GetReadyWork(b.ctx, filter)
		if err != nil {
			continue
		}
		// Filter out blocked issues
		for _, issue := range issues {
			// Fetch labels if not already populated
			if issue.Labels == nil {
				labels, err := m.store.GetLabels(b.ctx, issue.ID)
				if err == nil {
					issue.Labels = labels
				}
			}
			blockers, err := b.getBlockersFromStore(m.store, issue.ID)
			if err != nil || len(blockers) == 0 {
				allReady = append(allReady, TaskWithMount{
					Issue: issue,
					Mount: name,
					Cwd:   m.cwd,
				})
			}
		}
	}
	return json.MarshalIndent(allReady, "", "  ")
}


func (b *BeadsFS) readSearchFromStore(store bd.Storage, query string) ([]byte, error) {
	issues, err := store.SearchIssues(b.ctx, query, bd.IssueFilter{})
	if err != nil {
		return nil, err
	}
	if issues == nil {
		issues = []*bd.Issue{}
	}
	return json.MarshalIndent(issues, "", "  ")
}


func (b *BeadsFS) readByExternalRefFromStore(store bd.Storage, ref string) ([]byte, error) {
	issue, err := store.GetIssueByExternalRef(b.ctx, ref)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(issue, "", "  ")
}




func (b *BeadsFS) readBatchFromStore(store bd.Storage, ids string) ([]byte, error) {
	idList := strings.Split(ids, ",")
	issues, err := store.GetIssuesByIDs(b.ctx, idList)
	if err != nil {
		return nil, err
	}
	if issues == nil {
		issues = []*bd.Issue{}
	}
	return json.MarshalIndent(issues, "", "  ")
}


func (b *BeadsFS) readByLabelFromStore(store bd.Storage, label string) ([]byte, error) {
	issues, err := store.GetIssuesByLabel(b.ctx, label)
	if err != nil {
		return nil, err
	}
	if issues == nil {
		issues = []*bd.Issue{}
	}
	return json.MarshalIndent(issues, "", "  ")
}


func (b *BeadsFS) readBlockedFromStore(store bd.Storage) ([]byte, error) {
	filter := bd.WorkFilter{}
	blocked, err := store.GetBlockedIssues(b.ctx, filter)
	if err != nil {
		return nil, err
	}
	if blocked == nil {
		blocked = []*bd.BlockedIssue{}
	}
	return json.MarshalIndent(blocked, "", "  ")
}

// readPending returns beads in pending_approval or pending_review status,
// i.e. those awaiting a human APPROVAL_RESPONSE or REVIEW_RESPONSE.

func (b *BeadsFS) readPendingFromStore(store bd.Storage) ([]byte, error) {
	issues, err := store.SearchIssues(b.ctx, "", bd.IssueFilter{})
	if err != nil {
		return nil, err
	}
	pending := []*bd.Issue{}
	for _, issue := range issues {
		if issue.Status == StatusPendingApproval || issue.Status == StatusPendingReview {
			pending = append(pending, issue)
		}
	}
	return json.MarshalIndent(pending, "", "  ")
}


func (b *BeadsFS) readDependenciesMetaFromStore(store bd.Storage, beadID string) ([]byte, error) {
	deps, err := store.GetDependenciesWithMetadata(b.ctx, beadID)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(deps, "", "  ")
}


func (b *BeadsFS) readDependentsMetaFromStore(store bd.Storage, beadID string) ([]byte, error) {
	deps, err := store.GetDependentsWithMetadata(b.ctx, beadID)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(deps, "", "  ")
}


func (b *BeadsFS) readChildrenFromStore(store bd.Storage, parentID string) ([]byte, error) {
	filter := bd.IssueFilter{ParentID: &parentID}
	children, err := store.SearchIssues(b.ctx, "", filter)
	if err != nil {
		return nil, err
	}
	if children == nil {
		children = []*bd.Issue{}
	}
	return json.MarshalIndent(children, "", "  ")
}


func (b *BeadsFS) readStaleFromStore(store bd.Storage) ([]byte, error) {
	// Get all non-closed issues
	filter := bd.IssueFilter{}
	issues, err := store.SearchIssues(b.ctx, "", filter)
	if err != nil {
		return nil, err
	}
	
	// Filter to stale issues (not updated in 30+ days, status open or in_progress)
	cutoff := time.Now().AddDate(0, 0, -30)
	stale := []*bd.Issue{}
	for _, issue := range issues {
		if issue.Status != bd.StatusClosed && issue.UpdatedAt.Before(cutoff) {
			stale = append(stale, issue)
		}
	}
	
	return json.MarshalIndent(stale, "", "  ")
}


func (b *BeadsFS) readQuery() ([]byte, error) {
	if b.queryResult == nil {
		return json.MarshalIndent([]*bd.Issue{}, "", "  ")
	}
	return json.MarshalIndent(b.queryResult, "", "  ")
}



func (b *BeadsFS) readBeadPropertyFromStore(store bd.Storage, beadID, property string) ([]byte, error) {
	issue, err := store.GetIssue(b.ctx, beadID)
	if err != nil {
		return nil, err
	}
	if issue == nil {
		return nil, fmt.Errorf("bead not found: %s", beadID)
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
			Blockers        []string `json:"blockers,omitempty"`
			CapabilityLevel string   `json:"capability_level,omitempty"`
		}
		result := BeadWithBlockers{Issue: issue}
		if blockers, err := b.getBlockersFromStore(store, beadID); err == nil {
			result.Blockers = blockers
		}
		result.CapabilityLevel = extractCapabilityLevel(issue.Labels)
		return json.MarshalIndent(result, "", "  ")
	case "comments":
		comments, err := store.GetIssueComments(b.ctx, beadID)
		if err != nil {
			return nil, err
		}
		return json.MarshalIndent(comments, "", "  ")
	case "labels":
		labels, err := store.GetLabels(b.ctx, beadID)
		if err != nil {
			return nil, err
		}
		return json.MarshalIndent(labels, "", "  ")
	case "dependents":
		dependents, err := store.GetDependents(b.ctx, beadID)
		if err != nil {
			return nil, err
		}
		return json.MarshalIndent(dependents, "", "  ")
	case "tree":
		tree, err := store.GetDependencyTree(b.ctx, beadID, 10, false, false)
		if err != nil {
			return nil, err
		}
		return json.MarshalIndent(tree, "", "  ")
	case "events":
		events, err := store.GetEvents(b.ctx, beadID, 100)
		if err != nil {
			return nil, err
		}
		return json.MarshalIndent(events, "", "  ")
	default:
		return nil, fmt.Errorf("unknown property: %s", property)
	}
}


func (b *BeadsFS) getBlockersFromStore(store bd.Storage, id string) ([]string, error) {
	deps, err := store.GetDependenciesWithMetadata(b.ctx, id)
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


func (b *BeadsFS) executeCtlOnStore(store bd.Storage, cmd string) error {
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
		return store.SetConfig(b.ctx, "issue_prefix", prefix)
		
	case "new", "create":
		if len(args) < 1 {
			return fmt.Errorf("usage: new 'title' ['description'] [parent-id] [--no-lint] [capability=low|standard|high]")
		}

		// Strip --no-lint flag and capability= option from args before positional parsing.
		noLint := false
		capLevel := ""
		filtered := args[:0]
		for _, a := range args {
			if a == "--no-lint" {
				noLint = true
			} else if level, ok := strings.CutPrefix(a, "capability="); ok {
				capLevel = level
			} else {
				filtered = append(filtered, a)
			}
		}
		args = filtered

		title := args[0]
		description := ""
		parentID := ""
		if len(args) > 1 {
			description = args[1]
		}
		if len(args) > 2 {
			parentID = args[2]
		}

		// Emit lint warnings unless --no-lint or issue_type=idea.
		// The new command always creates TypeTask; idea exemption applies
		// when the title starts with "IDEA:" as a convention.
		isIdea := strings.HasPrefix(strings.ToUpper(title), "IDEA:")
		if !noLint && !isIdea && description != "" {
			for _, w := range lintDescription(description) {
				logging.Logger().Warn("lint warning", zap.String("title", title), zap.String("warning", w))
			}
		}

		if parentID != "" {
			childID, err := b.createSubtaskOnStore(store, parentID, title, description, actor)
			if err != nil {
				return err
			}
			// Apply capability label if provided.
			if capLevel != "" {
				_ = store.AddLabel(b.ctx, childID, capabilityPrefix+capLevel, actor)
			}
			return nil
		}

		issue := &bd.Issue{
			Title:       title,
			Description: description,
			Status:      bd.StatusOpen,
			IssueType:   bd.TypeTask,
			Priority:    2,
		}
		if capLevel != "" {
			issue.Labels = []string{capabilityPrefix + capLevel}
		}
		return store.CreateIssue(b.ctx, issue, actor)
		
	case "claim":
		if len(args) < 1 {
			return fmt.Errorf("usage: claim <bead-id> [assignee]")
		}
		assignee := "user"
		if len(args) > 1 {
			assignee = args[1]
		}
		updates := map[string]any{
			"assignee": assignee,
			"status":   bd.StatusInProgress,
		}
		return store.UpdateIssue(b.ctx, args[0], updates, actor)
		
	case "complete", "close":
		if len(args) < 1 {
			return fmt.Errorf("usage: complete <bead-id>")
		}
		if err := store.UpdateIssue(b.ctx, args[0], map[string]any{"assignee": ""}, actor); err != nil {
			return err
		}
		return store.CloseIssue(b.ctx, args[0], "completed", actor, "")

	case "fail":
		if len(args) < 2 {
			return fmt.Errorf("usage: fail <bead-id> 'reason'")
		}
		if err := store.UpdateIssue(b.ctx, args[0], map[string]any{"assignee": ""}, actor); err != nil {
			return err
		}
		return store.CloseIssue(b.ctx, args[0], args[1], actor, "")
		
	case "dep", "add-dep":
		if len(args) < 2 {
			return fmt.Errorf("usage: dep <child-id> <parent-id>")
		}
		dep := &bd.Dependency{
			IssueID:     args[0],
			DependsOnID: args[1],
			Type:        bd.DepBlocks,
		}
		return store.AddDependency(b.ctx, dep, actor)

	case "undep", "rm-dep":
		if len(args) < 2 {
			return fmt.Errorf("usage: undep <child-id> <parent-id>")
		}
		return store.RemoveDependency(b.ctx, args[0], args[1], actor)

	case "update":
		if len(args) < 3 {
			return fmt.Errorf("usage: update <bead-id> <field> 'value'")
		}
		updates := map[string]any{
			args[1]: args[2],
		}
		return store.UpdateIssue(b.ctx, args[0], updates, actor)

	case "delete", "rm":
		if len(args) < 1 {
			return fmt.Errorf("usage: delete <bead-id>")
		}
		return store.DeleteIssue(b.ctx, args[0])

	case "comment":
		if len(args) < 2 {
			return fmt.Errorf("usage: comment <bead-id> 'text'")
		}
		_, err := store.AddIssueComment(b.ctx, args[0], actor, args[1])
		return err

	case "label":
		if len(args) < 2 {
			return fmt.Errorf("usage: label <bead-id> 'label'")
		}
		return store.AddLabel(b.ctx, args[0], args[1], actor)

	case "unlabel":
		if len(args) < 2 {
			return fmt.Errorf("usage: unlabel <bead-id> 'label'")
		}
		return store.RemoveLabel(b.ctx, args[0], args[1], actor)

	case "set-capability":
		// Convenience command that replaces any existing capability label with
		// the given level (low, standard, or high).
		// Usage: set-capability <bead-id> low|standard|high
		if len(args) < 2 {
			return fmt.Errorf("usage: set-capability <bead-id> low|standard|high")
		}
		beadID := args[0]
		level := args[1]
		if level != "low" && level != "standard" && level != "high" {
			return fmt.Errorf("invalid capability level %q: must be low, standard, or high", level)
		}
		// Remove any existing capability labels first.
		existing, err := store.GetLabels(b.ctx, beadID)
		if err != nil {
			return err
		}
		for _, lbl := range existing {
			if level, ok := strings.CutPrefix(lbl, capabilityPrefix); ok && level != "" {
				_ = store.RemoveLabel(b.ctx, beadID, lbl, actor)
			}
		}
		return store.AddLabel(b.ctx, beadID, capabilityPrefix+level, actor)

	case "batch-create":
		if len(args) < 1 {
			return fmt.Errorf("usage: batch-create <json-array>")
		}
		var issues []*bd.Issue
		if err := json.Unmarshal([]byte(args[0]), &issues); err != nil {
			return fmt.Errorf("invalid JSON: %w", err)
		}
		return store.CreateIssues(b.ctx, issues, actor)

	// Approval gate commands.
	// Bots should call these after sending an APPROVAL_REQUEST or REVIEW_REQUEST
	// to the user.  The human responds via the inbox UI; on approve/reject the
	// bot calls "resume" or "fail" to continue or abort work.

	case "pending-approval":
		// Atomically set status=pending_approval and assignee (defaults to "user").
		// Usage: pending-approval <bead-id> [assignee]
		// Canonical workflow:
		//   1. Bot sends APPROVAL_REQUEST to user mailbox
		//   2. Bot calls: echo "pending-approval <id>" | 9p write agent/beads/ctl
		//   3. Human approves → bot calls: echo "resume <id> <bot-id>" | 9p write agent/beads/ctl
		//   4. Human rejects → bot calls: echo "fail <id> 'rejected'" | 9p write agent/beads/ctl
		if len(args) < 1 {
			return fmt.Errorf("usage: pending-approval <bead-id> [assignee]")
		}
		assignee := "user"
		if len(args) > 1 {
			assignee = args[1]
		}
		updates := map[string]any{
			"status":   StatusPendingApproval,
			"assignee": assignee,
		}
		return store.UpdateIssue(b.ctx, args[0], updates, actor)

	case "pending-review":
		// Atomically set status=pending_review and assignee (defaults to "user").
		// Usage: pending-review <bead-id> [assignee]
		// Canonical workflow:
		//   1. Bot sends REVIEW_REQUEST to user mailbox
		//   2. Bot calls: echo "pending-review <id>" | 9p write agent/beads/ctl
		//   3. Human responds → bot calls: echo "resume <id> <bot-id>" | 9p write agent/beads/ctl
		//   4. Human rejects → bot calls: echo "fail <id> 'review failed'" | 9p write agent/beads/ctl
		if len(args) < 1 {
			return fmt.Errorf("usage: pending-review <bead-id> [assignee]")
		}
		assignee := "user"
		if len(args) > 1 {
			assignee = args[1]
		}
		updates := map[string]any{
			"status":   StatusPendingReview,
			"assignee": assignee,
		}
		return store.UpdateIssue(b.ctx, args[0], updates, actor)

	case "resume":
		// Atomically set status=in_progress and assignee after human approval/review.
		// Usage: resume <bead-id> [assignee]
		// Assignee defaults to "user"; pass the bot's agent ID to hand work back to a bot.
		// Use "fail" instead if the human rejected.
		if len(args) < 1 {
			return fmt.Errorf("usage: resume <bead-id> [assignee]")
		}
		assignee := "user"
		if len(args) > 1 {
			assignee = args[1]
		}
		updates := map[string]any{
			"status":   bd.StatusInProgress,
			"assignee": assignee,
		}
		return store.UpdateIssue(b.ctx, args[0], updates, actor)

	case "unclaim":
		// Atomically clear assignee and reset status to open.
		// Usage: unclaim <bead-id>
		if len(args) < 1 {
			return fmt.Errorf("usage: unclaim <bead-id>")
		}
		updates := map[string]any{
			"assignee": "",
			"status":   bd.StatusOpen,
		}
		return store.UpdateIssue(b.ctx, args[0], updates, actor)

	default:
		return fmt.Errorf("unknown command: %s (supported: init, new, update, delete, claim, unclaim, complete, fail, dep, undep, pending-approval, pending-review, resume, label, unlabel, set-capability)", command)
	}
}

// lintDescription checks a bead description for quality signals and returns
// a list of warning messages. An empty slice means the description passed.
//
// Rules enforced:
//   - File path present (.go/.py/etc or /internal/ etc.)
//   - Function name or precise location (foo(), func keyword, Acme address, L123)
//   - Minimum length (80 chars)
//   - Acceptance criterion keyword (should/must/returns/etc.)
//   - Acme address format (file:N,N not file:N-N)
//   - Imperative verb start (Fix/Add/Update/... not "Need to"/"Should")
//   - No vague language (somehow/maybe/etc.)
//   - "How" signal (following/same as/pattern from/...)
//   - No first-person voice (I need/we want/...)
//   - No forbidden vague phrases (fix this/update this/...)
//   - Inline code (backtick identifier required when file path present)
//   - Cross-reference on long descriptions (bd-XXX or URL for >150 chars)
func lintDescription(description string) []string {
	var warnings []string
	lower := strings.ToLower(description)

	// --- Rule 1: File path signal ---
	hasFilePath := strings.Contains(description, ".go") ||
		strings.Contains(description, ".py") ||
		strings.Contains(description, ".ts") ||
		strings.Contains(description, ".js") ||
		strings.Contains(description, ".rs") ||
		strings.Contains(description, "/cmd/") ||
		strings.Contains(description, "/internal/") ||
		strings.Contains(description, "/src/") ||
		strings.Contains(description, "/pkg/")
	if !hasFilePath {
		warnings = append(warnings, "missing file path (add .go/.py/etc or /cmd//internal/ to help bots locate the code)")
	}

	// --- Rule 2: Function name or precise location ---
	// Matches: func calls (foo()), "func " keyword, Acme addresses (file:NNN or file:/re/),
	// explicit "line " text, or L123 / :123 style refs.
	hasFuncOrLine := strings.Contains(description, "()") ||
		strings.Contains(description, "func ") ||
		strings.Contains(lower, "line ") ||
		strings.Contains(lower, ":line") ||
		containsLineRef(description) ||
		containsAcmeRegexAddr(description)
	if !hasFuncOrLine {
		warnings = append(warnings, "missing function name or location (add func name, Acme address file.go:123, or L123)")
	}

	// --- Rule 3: Minimum length ---
	if len(description) < 80 {
		warnings = append(warnings, "description too short (aim for 80+ chars with What/Where/How/Accept)")
	}

	// --- Rule 4: Acceptance criterion keyword ---
	acceptKeywords := []string{"should", "returns", "displays", "must", "assert", "verify", "accept", "expect"}
	hasAccept := false
	for _, kw := range acceptKeywords {
		if strings.Contains(lower, kw) {
			hasAccept = true
			break
		}
	}
	if !hasAccept {
		warnings = append(warnings, "missing acceptance criterion (add: should/returns/must/accept)")
	}

	// --- Rule 5: Acme address format ---
	// file:NNN-NNN uses a hyphen range which is invalid in Acme/sam; use comma: file:NNN,NNN.
	if containsHyphenRange(description) {
		warnings = append(warnings, "invalid Acme address: use comma range file.go:123,125 not file.go:123-125")
	}

	// --- Rule 6: Imperative verb start ---
	// Warn if description begins with a known non-imperative pattern.
	nonImperativeStarters := []string{
		"need to ", "needs to ", "should ", "we need", "we want", "we should",
		"the ", "this ", "looking at", "looking into",
	}
	firstWordLower := strings.ToLower(strings.TrimSpace(description))
	for _, starter := range nonImperativeStarters {
		if strings.HasPrefix(firstWordLower, starter) {
			warnings = append(warnings, "start with imperative verb (Fix/Add/Update/Refactor) not '"+starter+"...'")
			break
		}
	}

	// --- Rule 7: No vague language ---
	vaguePhrases := []string{
		"somehow", " maybe ", "probably ", "try to ", " a bit ", " etc.", " etc,",
		"and so on", " stuff", "some kind of", "whatever", "sort of ", "kind of ",
	}
	for _, phrase := range vaguePhrases {
		if strings.Contains(lower, phrase) {
			warnings = append(warnings, "vague language '"+strings.TrimSpace(phrase)+"': replace with specific behavior")
			break
		}
	}

	// --- Rule 8: "How" signal (existing pattern reference) ---
	howSignals := []string{
		"following ", "pattern from", "same as ", "similar to ", "like in ",
		"mirrors ", "as in ", "modeled on", "following the ", "see ", "cf.",
	}
	hasHow := false
	for _, sig := range howSignals {
		if strings.Contains(lower, sig) {
			hasHow = true
			break
		}
	}
	if !hasHow {
		warnings = append(warnings, "missing 'how' signal (add: 'following pattern in X' or 'same as Y')")
	}

	// --- Rule 9: No first-person voice ---
	firstPersonPhrases := []string{
		"i need", "i want", "i think", "i'll ", "i will ", "i should",
		"we need", "we want", "we should", "we'll ", "we will ",
	}
	for _, fp := range firstPersonPhrases {
		if strings.Contains(lower, fp) {
			warnings = append(warnings, "avoid first-person ('"+strings.TrimSpace(fp)+"'): use imperative voice")
			break
		}
	}

	// --- Rule 10: Forbidden vague phrases ---
	forbiddenPhrases := []string{
		"fix this", "fix it", "update this", "make it work", "clean this up",
		"refactor this", "look at this", "deal with this", "handle this",
	}
	for _, fp := range forbiddenPhrases {
		if strings.Contains(lower, fp) {
			warnings = append(warnings, "forbidden vague phrase '"+fp+"': specify What/Where/How/Accept")
			break
		}
	}

	// --- Rule 11: Inline code (backtick identifier) ---
	// When a file path is present, at least one backtick-enclosed identifier is expected.
	if hasFilePath && !strings.Contains(description, "`") {
		warnings = append(warnings, "no inline code found: wrap identifiers in backticks (`funcName()`, `--flag`)")
	}

	// --- Rule 12: Cross-reference on long descriptions ---
	// Descriptions over 150 chars should reference a related bead or URL.
	if len(description) > 150 {
		hasBdRef := containsBdRef(description)
		hasURL := strings.Contains(lower, "http://") || strings.Contains(lower, "https://")
		if !hasBdRef && !hasURL {
			warnings = append(warnings, "long description missing cross-reference (add bd-XXX or URL in Refs)")
		}
	}

	return warnings
}

// containsLineRef returns true if s contains a line number reference
// in the form L<digits> (e.g. L385) or :<digits> (e.g. :385).
// Also recognises Acme character-position addresses: #<digits>.
func containsLineRef(s string) bool {
	for i := 0; i < len(s)-1; i++ {
		c := s[i]
		next := s[i+1]
		if (c == 'L' || c == ':' || c == '#') && next >= '0' && next <= '9' {
			return true
		}
	}
	return false
}

// containsAcmeRegexAddr returns true if s contains an Acme regex-style address
// of the form /word/ where word is at least 4 characters long
// (e.g. /lintDescription/ or /funcName/). Short slash-delimited tokens
// such as path components (/p9/, /cmd/, /src/) are intentionally excluded.
func containsAcmeRegexAddr(s string) bool {
	const minPatternLen = 4
	for i := 0; i < len(s)-2; i++ {
		if s[i] != '/' {
			continue
		}
		// Scan for closing slash; stop at whitespace or newline.
		j := i + 1
		for j < len(s) && s[j] != '/' && s[j] != ' ' && s[j] != '\n' {
			j++
		}
		if j < len(s) && s[j] == '/' && (j-i-1) >= minPatternLen {
			return true
		}
	}
	return false
}

// containsHyphenRange returns true if s contains an Acme-invalid hyphen range
// of the form file.ext:NNN-NNN (e.g. beads.go:123-125).
// The correct Acme syntax uses a comma: beads.go:123,125.
func containsHyphenRange(s string) bool {
	// Look for patterns like ":NNN-NNN" (colon, digits, hyphen, digits).
	for i := 0; i < len(s); i++ {
		if s[i] != ':' {
			continue
		}
		i++
		// Consume leading digits.
		if i >= len(s) || s[i] < '0' || s[i] > '9' {
			continue
		}
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		// Expect hyphen followed by digits.
		if i < len(s)-1 && s[i] == '-' && s[i+1] >= '0' && s[i+1] <= '9' {
			return true
		}
	}
	return false
}

// containsBdRef returns true if s contains a bead cross-reference of the
// form "bd-" followed by one or more lowercase alphanumeric characters.
func containsBdRef(s string) bool {
	lower := strings.ToLower(s)
	idx := strings.Index(lower, "bd-")
	for idx != -1 {
		if idx+3 < len(lower) && isAlphanumeric(lower[idx+3]) {
			return true
		}
		next := strings.Index(lower[idx+1:], "bd-")
		if next == -1 {
			break
		}
		idx = idx + 1 + next
	}
	return false
}

// isAlphanumeric returns true if b is a lowercase letter or digit.
func isAlphanumeric(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9')
}


func (b *BeadsFS) createSubtaskOnStore(store bd.Storage, parentID, title, description, actor string) (string, error) {
	// Verify parent exists
	parent, err := store.GetIssue(b.ctx, parentID)
	if err != nil {
		return "", fmt.Errorf("failed to get parent: %w", err)
	}
	if parent == nil {
		return "", fmt.Errorf("parent %s not found", parentID)
	}

	// Find next child number by scanning existing IDs with parent filter
	filter := bd.IssueFilter{ParentID: &parentID}
	children, err := store.SearchIssues(b.ctx, "", filter)
	if err != nil {
		return "", fmt.Errorf("failed to search children: %w", err)
	}

	nextChild := 1
	prefix := parentID + "."
	for _, issue := range children {
		if suffix, ok := strings.CutPrefix(issue.ID, prefix); ok {
			if !strings.Contains(suffix, ".") {
				if n, err := strconv.Atoi(suffix); err == nil && n >= nextChild {
					nextChild = n + 1
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

	if err := store.CreateIssue(b.ctx, issue, actor); err != nil {
		return "", err
	}
	return childID, nil
}

// extractCapabilityLevel scans a label slice and returns the level portion
// (low, standard, or high) of the first "capability:<level>" label found.
// Returns "" if no capability label is present or level is invalid.
func extractCapabilityLevel(labels []string) string {
	for _, lbl := range labels {
		if level, ok := strings.CutPrefix(lbl, capabilityPrefix); ok && level != "" {
			if level == "low" || level == "standard" || level == "high" {
				return level
			}
		}
	}
	return ""
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
	for _, c := range s {
		if quoteChar != 0 {
			// Inside quotes
			if c == quoteChar {
				quoteChar = 0 // End quote
				wasQuoted = true
			} else {
				current.WriteRune(c)
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
				current.WriteRune(c)
			}
		}
	}
	
	if current.Len() > 0 || wasQuoted {
		args = append(args, current.String())
	}
	
	return args
}
