---
name: beads
description: Manage tasks using the beads 9P interface. Use when creating, updating, listing, or deleting tasks/beads, or managing task dependencies.
---

# Beads Task System

## Purpose

Beads is a hierarchical task tracking system accessed via 9P. Use it to track work items, their status, and dependencies. All operations persist immediately to a Dolt database, providing crash resilience and resumability.

## When to Use

- Creating or managing tasks
- Tracking work progress
- Setting up task hierarchies (parent/child)
- Managing blockers/dependencies
- Finding ready (unblocked) work

## Atomic Operations

Beads emphasizes atomic, single-purpose operations via the 9P control file:

### Claiming Work
```bash
# Atomically sets assignee and status to in_progress
echo "claim bd-abc" | 9p write agent/beads/ctl
```

### Completing Work
```bash
# Atomically closes the bead
echo "complete bd-abc" | 9p write agent/beads/ctl
```

### Single-Purpose Commands
Each control command does exactly one thing:
- `new` - Create a bead
- `claim` - Claim a bead (sets assignee + status)
- `complete` - Complete a bead
- `fail` - Fail a bead with reason
- `update` - Update a single field
- `dep` - Add a dependency
- `undep` - Remove a dependency
- `delete` - Delete a bead
- `comment` - Add a comment
- `label` - Add a label
- `unlabel` - Remove a label

### Immediate Feedback
After write operations, read back state to confirm:
```bash
echo "claim bd-abc" | 9p write agent/beads/ctl
9p read agent/beads/bd-abc/json | jq .status
# Output: "in_progress"
```

### No Batch Operations
Process beads one at a time in sequence:
```bash
# Correct: Sequential atomic operations
echo "claim bd-abc" | 9p write agent/beads/ctl
# ... do work ...
echo "complete bd-abc" | 9p write agent/beads/ctl

# Incorrect: Don't try to batch multiple beads
```

## Dependency-Aware Workflow

### Finding Ready Work
Use the `ready` endpoint to get beads with no blockers:
```bash
9p read agent/beads/ready | jq
# Returns JSON array of unblocked beads
```

### Creating Dependencies
Add a dependency so parent blocks child:
```bash
# Child bd-xyz is blocked by parent bd-abc
echo "dep bd-xyz bd-abc" | 9p write agent/beads/ctl
```

### Hierarchical Tasks
Create children with automatic hierarchical IDs:
```bash
# Create parent
echo "new 'Implement feature' 'Add new capability'" | 9p write agent/beads/ctl
# Get parent ID (e.g., bd-abc)

# Create children - they get IDs bd-abc.1, bd-abc.2, etc.
echo "new 'Design API' 'Define interfaces' bd-abc" | 9p write agent/beads/ctl
echo "new 'Write tests' 'Add test coverage' bd-abc" | 9p write agent/beads/ctl
```

### Checking Blockers
Read the blockers field to see what's blocking a bead:
```bash
9p read agent/beads/bd-abc/json | jq .blockers
# Output: ["bd-abc.1", "bd-abc.2"]
```

### Recommended Workflow
1. Check ready work: `9p read agent/beads/ready`
2. Claim an unblocked bead: `echo "claim bd-xyz" | 9p write agent/beads/ctl`
3. Do the work
4. Complete it: `echo "complete bd-xyz" | 9p write agent/beads/ctl`
5. Completing a bead unblocks its dependents

## JSON Output for Programmatic Use

All read endpoints return JSON for easy parsing:

### Available Endpoints
- `agent/beads/list` - All beads
- `agent/beads/ready` - Beads with no blockers
- `agent/beads/blocked` - Beads that have blockers
- `agent/beads/stale` - Beads 30+ days old
- `agent/beads/stats` - Statistics (counts by status)
- `agent/beads/<id>/json` - Single bead with full details

### Structured Fields
Each bead JSON contains:
```json
{
  "id": "bd-abc",
  "title": "Task title",
  "description": "Detailed description",
  "status": "open",
  "priority": 2,
  "issue_type": "task",
  "assignee": "agent-123",
  "blockers": ["bd-abc.1", "bd-abc.2"],
  "created_at": "2026-02-20T21:00:00Z",
  "updated_at": "2026-02-20T21:30:00Z"
}
```

### Parsing with jq
Extract fields using jq:
```bash
# Get bead ID
9p read agent/beads/bd-abc/json | jq -r .id

# Get all blocker IDs
9p read agent/beads/bd-abc/json | jq -r .blockers[]

# Get status
9p read agent/beads/bd-abc/json | jq -r .status

# Get all ready bead IDs
9p read agent/beads/ready | jq -r .[].id
```

### Critical Rule
**NEVER parse human-readable output.** Only consume JSON from `9p read` endpoints.

## Persistence and Session Lifecycle

### Storage
Beads persist in a Dolt database at `.beads/` directory:
- MVCC (Multi-Version Concurrency Control)
- ACID transactions
- Version control built-in

### Crash Resilience
All operations immediately persist to the database. Agents can resume work after crashes without data loss.

### Session Workflow
1. **Start**: Read ready work
   ```bash
   9p read agent/beads/ready | jq
   ```

2. **Claim**: Atomically claim a bead
   ```bash
   echo "claim bd-abc" | 9p write agent/beads/ctl
   ```

3. **Work**: Perform the task

4. **Complete**: Atomically complete the bead
   ```bash
   echo "complete bd-abc" | 9p write agent/beads/ctl
   ```

### Event History
View the event history for a bead:
```bash
9p read agent/beads/bd-abc/events
# Returns JSON array of events (created, claimed, updated, completed)
```

### Critical Rule
**NEVER manually access `.beads/` directory.** Always use the 9P interface. Direct database modification breaks consistency.

## Anti-Patterns and Warnings

### ❌ NEVER manually edit `.beads/` directory
**Why**: Direct database modification breaks consistency and can corrupt data.
**Instead**: Use `9p write agent/beads/ctl` for all modifications.

### ❌ NEVER parse non-JSON output
**Why**: Human-readable formats change and are unreliable for programmatic use.
**Instead**: Only consume JSON from `9p read` endpoints.

### ❌ NEVER create markdown TODOs instead of beads
**Why**: TODOs in markdown are not tracked, searchable, or dependency-aware.
**Instead**: Use `echo "new 'title' 'desc'" | 9p write agent/beads/ctl`.

### ❌ NEVER bypass the 9P interface
**Why**: Direct SQL queries or Dolt commands bypass validation and event logging.
**Instead**: Always use `9p read` and `9p write agent/beads/ctl`.

### ❌ NEVER ignore blockers
**Why**: Working on blocked tasks wastes effort if dependencies aren't met.
**Instead**: Check `9p read agent/beads/ready` and respect dependency order.

### ❌ NEVER leave tasks unclaimed while working
**Why**: Other agents may claim the same work, causing conflicts.
**Instead**: Always `echo "claim bd-xxx" | 9p write agent/beads/ctl` before starting.

## Quick Command Reference

### List ready work
```bash
9p read agent/beads/ready | jq
```

### Create bead
```bash
echo "new \"Implement feature\" \"Add new capability\"" | 9p write agent/beads/ctl
```

### Create child bead
```bash
echo "new \"Design API\" \"Define interfaces\" bd-abc" | 9p write agent/beads/ctl
```

### Claim bead
```bash
echo "claim bd-abc" | 9p write agent/beads/ctl
```

### Complete bead
```bash
echo "complete bd-abc" | 9p write agent/beads/ctl
```

### Fail bead with reason
```bash
echo "fail bd-abc \"Cannot reproduce issue\"" | 9p write agent/beads/ctl
```

### Update field
```bash
echo "update bd-abc priority 1" | 9p write agent/beads/ctl
```

### Add dependency (parent blocks child)
```bash
echo "dep bd-xyz bd-abc" | 9p write agent/beads/ctl
```

### Remove dependency
```bash
echo "undep bd-xyz bd-abc" | 9p write agent/beads/ctl
```

### Check stale beads
```bash
9p read agent/beads/stale | jq
```

### Read single bead
```bash
9p read agent/beads/bd-abc/json | jq
```

### Add comment
```bash
echo "comment bd-abc \"Fixed the bug\"" | 9p write agent/beads/ctl
```

### Add label
```bash
echo "label bd-abc bug" | 9p write agent/beads/ctl
```

### Remove label
```bash
echo "unlabel bd-abc bug" | 9p write agent/beads/ctl
```

## Bead Fields

| Field | Type | Description |
|-------|------|-------------|
| id | string | Auto-generated (e.g., `bd-abc`, `bd-abc.1` for children) |
| title | string | Short task description |
| description | string | Detailed info |
| status | string | `open`, `in_progress`, `closed` |
| priority | int | 1-5 (1=highest) |
| assignee | string | Agent or user ID |
| blockers | array | IDs of blocking beads |
| issue_type | string | `task`, `bug`, etc. |
| created_at | string | ISO 8601 timestamp |
| updated_at | string | ISO 8601 timestamp |

## Complete Workflow Example

```bash
# Find ready work
9p read agent/beads/ready | jq

# Claim a bead
echo "claim bd-abc" | 9p write agent/beads/ctl

# Verify claim
9p read agent/beads/bd-abc/json | jq '.status, .assignee'

# Do the work...

# Complete the bead
echo "complete bd-abc" | 9p write agent/beads/ctl

# Verify completion
9p read agent/beads/bd-abc/json | jq .status
```
