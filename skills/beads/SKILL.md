---
name: beads
description: Manage tasks using the beads 9P interface. Use when creating, updating, listing, or deleting tasks/beads, or managing task dependencies.
---

# Beads Task System

## Purpose

Beads is a hierarchical task tracking system accessed via 9P. Use it to track work items, their status, and dependencies.

## When to Use

- Creating or managing tasks
- Tracking work progress
- Setting up task hierarchies (parent/child)
- Managing blockers/dependencies

## 9P Interface

### List all beads
```bash
9p read agent/beads/list
```
Returns JSON array of all beads.

### Read single bead
```bash
9p read agent/beads/{id}/json
```

### Control commands
Write to `agent/beads/ctl`:

```bash
# Create bead
echo "new 'title' 'description' [parent-id]" | 9p write agent/beads/ctl

# Update field
echo "update {id} {field} 'value'" | 9p write agent/beads/ctl

# Delete bead
echo "delete {id}" | 9p write agent/beads/ctl

# Add dependency (id is blocked by blocker-id)
echo "dep {id} {blocker-id}" | 9p write agent/beads/ctl

# Remove dependency
echo "undep {id} {blocker-id}" | 9p write agent/beads/ctl
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

## Hierarchical IDs

Child beads get hierarchical IDs based on parent:
- Parent: `bd-abc`
- Children: `bd-abc.1`, `bd-abc.2`, etc.
- Grandchildren: `bd-abc.1.1`, `bd-abc.1.2`, etc.

## CRITICAL: Task Lifecycle Rules

**ALWAYS follow this workflow when working on tasks:**

### Resolving Blocked Tasks
When asked to complete a task that has blockers:
1. **DO NOT STOP** - Immediately start working on the blocking subtasks
2. **Claim and complete each blocker** in sequence
3. **Then close the parent** once all blockers are resolved

**Example**: Asked to complete `bd-abc` which is blocked by `bd-abc.1` and `bd-abc.2`:
- Claim `bd-abc.1`, do the work, close it
- Claim `bd-abc.2`, do the work, close it  
- Close `bd-abc` (no longer blocked)

### Claiming Tasks
1. **If task has blockers**: Work on the blockers first (don't just report them!)
2. **CLAIM the task** - Set status to `in_progress` BEFORE starting work
3. **For tasks with subtasks**: Claim EACH subtask as you work on it

### Completing Tasks
1. **CLOSE each subtask** immediately after completing it
2. **CLOSE the parent** only after ALL subtasks are closed
3. **Never leave tasks unclaimed while working on them**

```bash
# CLAIM - Do this BEFORE starting work
echo "update bd-xxx status in_progress" | 9p write agent/beads/ctl

# DO THE WORK...

# CLOSE - Do this IMMEDIATELY after completing
echo "update bd-xxx status closed" | 9p write agent/beads/ctl
```

## Examples

```bash
# Create root task
echo "new 'Implement feature X' 'Add new capability'" | 9p write agent/beads/ctl

# Get its ID
ID=$(9p read agent/beads/list | jq -r '.[-1].id')

# Create subtasks (these will block the parent)
echo "new 'Design API' '' $ID" | 9p write agent/beads/ctl
echo "new 'Write tests' '' $ID" | 9p write agent/beads/ctl

# Work on subtasks FIRST (parent is blocked)
echo "update $ID.1 status in_progress" | 9p write agent/beads/ctl
# ... do design work ...
echo "update $ID.1 status closed" | 9p write agent/beads/ctl

echo "update $ID.2 status in_progress" | 9p write agent/beads/ctl
# ... write tests ...
echo "update $ID.2 status closed" | 9p write agent/beads/ctl

# NOW close the parent (all children done)
echo "update $ID status closed" | 9p write agent/beads/ctl
```
