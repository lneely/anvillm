// Package beads provides storage wrapper for steveyegge/beads.
package beads

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	bd "github.com/steveyegge/beads"
)

// Re-export types
type (
	Issue       = bd.Issue
	IssueFilter = bd.IssueFilter
	Status      = bd.Status
)

// Store wraps beads storage.
type Store struct {
	store bd.Storage
	ctx   context.Context
}

// NewStore creates a beads store.
func NewStore(ctx context.Context, beadsDir string) (*Store, error) {
	store, err := bd.OpenFromConfig(ctx, beadsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to open beads: %w", err)
	}
	return &Store{store: store, ctx: ctx}, nil
}

// Close closes the store.
func (s *Store) Close() error {
	if s.store != nil {
		return s.store.Close()
	}
	return nil
}

// Init initializes the beads database with a prefix.
func (s *Store) Init(prefix string) error {
	return s.store.SetConfig(s.ctx, "issue_prefix", prefix)
}


// CreateBead creates a bead.
func (s *Store) CreateBead(title, role, description, actor string) (string, error) {
	issue := &bd.Issue{
		Title:       title,
		Description: description,
		Status:      bd.StatusOpen,
		IssueType:   bd.TypeTask,
		Priority:    2,
	}
	
	if err := s.store.CreateIssue(s.ctx, issue, actor); err != nil {
		return "", err
	}
	return issue.ID, nil
}

// ClaimBead claims a bead.
func (s *Store) ClaimBead(id, actor string) error {
	updates := map[string]interface{}{
		"assignee": actor,
		"status":   bd.StatusInProgress,
	}
	return s.store.UpdateIssue(s.ctx, id, updates, actor)
}

// CompleteBead completes a bead.
func (s *Store) CompleteBead(id, actor string) error {
	return s.store.CloseIssue(s.ctx, id, "completed", actor, "")
}

// FailBead fails a bead.
func (s *Store) FailBead(id, reason, actor string) error {
	return s.store.CloseIssue(s.ctx, id, reason, actor, "")
}

// GetBead gets a bead.
func (s *Store) GetBead(id string) (*bd.Issue, error) {
	return s.store.GetIssue(s.ctx, id)
}

// ListBeads lists beads.
func (s *Store) ListBeads(filter bd.IssueFilter) ([]*bd.Issue, error) {
	return s.store.SearchIssues(s.ctx, "", filter)
}

// ReadyBeads returns ready beads.
func (s *Store) ReadyBeads(role string) ([]*bd.Issue, error) {
	status := bd.StatusOpen
	filter := bd.IssueFilter{
		Status: &status,
	}
	
	issues, err := s.store.SearchIssues(s.ctx, "", filter)
	if err != nil {
		return nil, err
	}
	
	ready := make([]*bd.Issue, 0)
	for _, issue := range issues {
		deps, err := s.store.GetDependencies(s.ctx, issue.ID)
		if err != nil {
			continue
		}
		
		hasOpenDeps := false
		for _, dep := range deps {
			if dep.Status != bd.StatusClosed {
				hasOpenDeps = true
				break
			}
		}
		
		if !hasOpenDeps {
			ready = append(ready, issue)
		}
	}
	
	return ready, nil
}

// GetBlockers returns IDs of issues that block the given issue.
func (s *Store) GetBlockers(id string) ([]string, error) {
	deps, err := s.store.GetDependencies(s.ctx, id)
	if err != nil {
		return nil, err
	}
	var blockers []string
	for _, dep := range deps {
		if dep.Status != bd.StatusClosed {
			blockers = append(blockers, dep.ID)
		}
	}
	return blockers, nil
}

// AddDependency adds a dependency.
func (s *Store) AddDependency(childID, parentID, actor string) error {
	dep := &bd.Dependency{
		IssueID:     childID,
		DependsOnID: parentID,
		Type:        bd.DepBlocks,
	}
	return s.store.AddDependency(s.ctx, dep, actor)
}

// RemoveDependency removes a dependency.
func (s *Store) RemoveDependency(childID, parentID, actor string) error {
	return s.store.RemoveDependency(s.ctx, childID, parentID, actor)
}

// UpdateBead updates a field on a bead.
func (s *Store) UpdateBead(id, field, value, actor string) error {
	updates := map[string]interface{}{
		field: value,
	}
	return s.store.UpdateIssue(s.ctx, id, updates, actor)
}

// DeleteBead deletes a bead.
func (s *Store) DeleteBead(id string) error {
	return s.store.DeleteIssue(s.ctx, id)
}

// CreateSubtask creates a subtask with hierarchical ID (e.g., bd-s2r.1).
func (s *Store) CreateSubtask(parentID, title, description, actor string) (string, error) {
	// Verify parent exists
	parent, err := s.store.GetIssue(s.ctx, parentID)
	if err != nil {
		return "", fmt.Errorf("failed to get parent: %w", err)
	}
	if parent == nil {
		return "", fmt.Errorf("parent %s not found", parentID)
	}

	// Find next child number by scanning existing IDs
	nextChild := 1
	issues, err := s.store.SearchIssues(s.ctx, "", bd.IssueFilter{})
	if err == nil {
		prefix := parentID + "."
		for _, issue := range issues {
			if strings.HasPrefix(issue.ID, prefix) {
				// Extract child number from ID like "bd-s2r.3"
				suffix := strings.TrimPrefix(issue.ID, prefix)
				// Handle nested children - only count direct children
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

	if err := s.store.CreateIssue(s.ctx, issue, actor); err != nil {
		return "", err
	}

	// Add parent-child dependency
	dep := &bd.Dependency{
		IssueID:     childID,
		DependsOnID: parentID,
		Type:        bd.DepParentChild,
	}
	if err := s.store.AddDependency(s.ctx, dep, actor); err != nil {
		// Issue created but dep failed - not fatal
		return childID, nil
	}

	return childID, nil
}

