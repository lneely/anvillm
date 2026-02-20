---
name: github-to-beads
description: Import GitHub issues and PRs into beads task tracking. Use when user asks to pull GitHub issue info, define work from an issue, or create tasks from GitHub.
---

# GitHub to Beads

## Purpose

Import GitHub issue hierarchies into beads, creating a task tree for agent work tracking.

## When to Use

- User asks to "pull" or "import" a GitHub issue or PR
- User wants to define work based on a GitHub issue
- User asks to create beads/tasks from GitHub

## When NOT to Use

- User just wants to view issue info (use `gh issue view` directly)
- User wants to modify GitHub issues (this is pull-only)
- General bead management (use `beads` skill)

## GitHub CLI

Use the `gh` CLI:

```bash
# View issue (human readable)
gh issue view 123 --repo owner/repo

# View issue (JSON for parsing)
gh issue view 123 --repo owner/repo --json number,title,body,state,labels,assignees

# List linked issues (via API)
gh api repos/owner/repo/issues/123 --jq '.body'
```

## Import Process

### 1. Check Existing Beads

```bash
9p read agent/beads/list | grep "#123"
```
Skip if already tracked.

### 2. Parse Issue Body for Task Lists

GitHub issues may contain task lists in markdown:
```markdown
- [ ] Task 1
- [x] Task 2 (completed)
- [ ] Task 3
```

Parse unchecked items as subtasks.

### 3. Create Beads Top-Down

Create parent issue first, then subtasks with parent ID. See `beads` skill for commands.

**CRITICAL**: Include issue number in title: `#123: <title>`

## Field Mapping

| GitHub Field | Bead Field | Notes |
|--------------|------------|-------|
| number + title | title | Format: `#123: title` |
| body | description | **MUST include issue body** |
| - | status | `open` for open issues, `closed` for closed |
| labels | issue_type | Map `bug` label to `bug`, default to `task` |

**CRITICAL**: The bead description MUST contain the GitHub issue body. This provides essential context for working on the task.

## Parsing Task Lists

Check issue body for task lists. Create as children of that issue's bead.

**Create subtasks for:**
- `- [ ]` unchecked checkboxes

**Skip:**
- `- [x]` checked boxes (done)

## Example

```bash
# Fetch issue
gh issue view 123 --repo owner/repo --json number,title,body,state

# Create parent bead
echo "new '#123: Add new feature' 'Issue body here'" | 9p write agent/beads/ctl

# Get parent ID
ROOT=$(9p read agent/beads/list | jq -r '.[] | select(.title | contains("#123")) | .id')

# Create subtasks from task list
echo "new 'Implement API endpoint' '' $ROOT" | 9p write agent/beads/ctl
echo "new 'Add tests' '' $ROOT" | 9p write agent/beads/ctl
```

## Human Review

After import, reviewer may:
- Remove beads bot doesn't need
- Adjust descriptions
- Add context
