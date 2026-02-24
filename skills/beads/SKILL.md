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
# MUST pass your agent ID (available as $AGENT_ID)
echo "claim bd-abc $AGENT_ID" | 9p write agent/beads/ctl
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
Read the blockers field to see explicit dependencies blocking a bead:
```bash
9p read agent/beads/bd-xyz/json | jq .blockers
# Output: ["bd-abc"] - only shows explicit dependencies added via "dep" command
```

**Important**: The blockers field only contains beads added as explicit dependencies using `echo "dep <child> <parent>" | 9p write agent/beads/ctl`. Child beads do NOT appear in the parent's blockers field.

### Checking Outstanding Child Work
To determine if a parent bead has incomplete child tasks, query the children endpoint and filter by status:
```bash
# Get all non-closed children of bd-abc
9p read agent/beads/children/bd-abc | jq '[.[] | select(.status != "closed")]'

# Count open children
9p read agent/beads/children/bd-abc | jq '[.[] | select(.status != "closed")] | length'

# Check if any children remain open
9p read agent/beads/children/bd-abc | jq 'any(.status != "closed")'
```

### Recommended Workflow
1. Check ready work: `9p read agent/beads/ready`
2. Claim an unblocked bead: `echo "claim bd-xyz $AGENT_ID" | 9p write agent/beads/ctl`
3. Search KB for relevant context: `grep -ril "<keyword>" ~/doc/agent-kb/`
4. Do the work (using KB-sourced file paths and patterns where available)
5. Before completing, check for child beads: `9p read agent/beads/children/bd-xyz | jq .`
6. If children exist: claim, work on, and complete each child before proceeding (see [Hierarchical Completion Protocol](#hierarchical-completion-protocol))
7. Verify all children closed: `9p read agent/beads/children/bd-xyz | jq 'all(.status == "closed")'`
8. Only then complete the bead: `echo "complete bd-xyz" | 9p write agent/beads/ctl`
9. Completing a bead unblocks its dependents

## Leaving Context for Future Agents

Beads serve as dynamic/temporal memory for agent execution. Beyond status tracking, they capture decisions, dead ends, discoveries, and rationale — context that helps future agents working on related tasks avoid repeated mistakes and build on prior work.

This is distinct from `agent-kb` (stable, reusable architectural knowledge). Beads hold *execution-time context*: what was tried, what failed, what was discovered, and why a particular approach was chosen.

### When to Comment

Comment on your bead as you work — not just at completion. Record:
- **Decisions**: "Chose approach Z because X failed due to Y."
- **Dead ends**: "Tried X, failed because Y. Avoided."
- **Discoveries**: "Found that endpoint /foo also rotates the refresh token."
- **Constraints**: "Avoided approach A because of constraint B (see bd-xyz)."

Comments may come from agents or humans (e.g., a human reviewing work, recording a constraint, or explaining a decision). Treat both equally as authoritative context.

```bash
echo "comment bd-abc \"Tried X first but hit rate limits. Switched to Y which batches requests.\"" | 9p write agent/beads/ctl
```

### When to Search Past Beads

Before starting any non-trivial task, search completed beads for related terms to surface prior decisions and gotchas:

```bash
9p read agent/beads/search/authentication | jq '.[].title'
9p read agent/beads/search/refresh-token | jq '.[].description'
```

When a bead looks relevant, read its full event history to see comments — this is where the most valuable context lives, including human-authored notes:

```bash
9p read agent/beads/bd-abc/events | jq '.[] | select(.type == "comment") | .body'
```

### What NOT to Do

- ❌ **NEVER create "knowledge beads"** solely to record information — this pollutes the task system with non-task entries
- ❌ Don't leave your bead with only status updates — rationale and discoveries are more valuable to future agents than "completed X"

Context belongs *in your task bead*, recorded as comments during execution.

## Description Quality and File Addressing

### What Makes a Good Description

Every bead description must enable a bot to complete the task cold — no codebase exploration, no guessing. The beads server enforces this with lint warnings emitted to stderr on `new`. A high-quality description:

- Starts with an **imperative verb**: Fix, Add, Update, Refactor, Implement, ...
- Names at least one **file path** with a recognized extension (.go, .py, .ts, .js, .rs) or path component (/internal/, /cmd/, /src/, /pkg/)
- Identifies a **precise location**: function name in backticks, or an Acme file address (see below)
- Wraps all identifiers in **backticks**: `` `lintDescription()` ``, `` `--no-lint` ``, `` `TypeName` ``
- Includes a **"how" signal**: "following the pattern in X", "same as Y", "mirrors Z"
- States an **observable acceptance criterion**: "must return ...", "should emit ...", "returns ..."
- Avoids **vague language**: somehow, maybe, etc., stuff, try to, a bit
- Avoids **first-person voice**: I need, we want, I'll
- For descriptions > 150 chars, includes a **cross-reference**: `bd-XXX` or a URL

### File Addressing

When referencing source locations, use **Acme/sam address syntax** — not the common but incorrect hyphen form.

**Correct forms (from `/home/lkn/plan9/plumb/fileaddr`):**

```
beads.go:123          line 123
beads.go:123,125      lines 123 to 125  (comma range)
beads.go:/funcName/   regex search      (pattern ≥ 4 chars)
beads.go:#4096        character offset  4096
```

**Never use:**

```
beads.go:123-125      ✗ hyphen range is invalid in Acme — use comma
```

The linter warns when it detects the hyphen form. The Conductor cold-completion gate also checks for correct Acme syntax before assigning P1/P2 beads.

### Lint Rules Summary

The server checks descriptions against these rules (warnings go to stderr; bypass with `--no-lint`):

| Rule | Signal checked |
|------|---------------|
| File path | `.go`/`.py`/etc. or `/internal/`/`/cmd/`/... |
| Location | `func()`, Acme address, `L123`, `:123`, `#123` |
| Minimum length | ≥ 80 characters |
| Acceptance criterion | should / must / returns / displays / assert / verify / accept / expect |
| Acme address format | No `file:N-N` hyphen ranges |
| Imperative start | Not "Need to" / "Should" / "We need" / "The X" |
| Vague language | No somehow / maybe / etc. / stuff / try to |
| How signal | following / same as / pattern from / similar to / mirrors |
| First-person | No I need / I want / we should / we'll |
| Forbidden phrases | No "fix this" / "update this" / "make it work" |
| Inline code | Backtick identifier required when file path present |
| Cross-reference | `bd-XXX` or URL required for descriptions > 150 chars |

## JSON Output for Programmatic Use

All read endpoints return JSON for easy parsing.

### Available Endpoints

**List Operations:**
- `agent/beads/list` - All beads
- `agent/beads/ready` - Beads with no blockers
- `agent/beads/blocked` - Beads that have blockers
- `agent/beads/stale` - Beads 30+ days old
- `agent/beads/stats` - Statistics (counts by status)

**Query Operations:**
- `agent/beads/query` - Query beads (write JSON filter, then read results)
- `agent/beads/search/<term>` - Search beads by term
- `agent/beads/by-ref/<ref>` - Find bead by external reference
- `agent/beads/label/<label>` - Beads with specific label
- `agent/beads/children/<id>` - Child beads of parent

**Bead Details:**
- `agent/beads/<id>/json` - Single bead with full details
- `agent/beads/<id>/events` - Event history
- `agent/beads/<id>/dependencies-meta` - Dependencies with metadata
- `agent/beads/<id>/dependents-meta` - Dependents with metadata

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
  "blockers": [],
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
   echo "claim bd-abc $AGENT_ID" | 9p write agent/beads/ctl
   ```

3. **Before You Start**: Search the knowledge base for relevant context before doing anything else
   ```bash
   # Extract keywords from the bead title and description (technical terms, file names, feature names)
   grep -ril "<keyword>" ~/doc/agent-kb/
   ```
   Read all matching entries in full. If KB has a file path or function name relevant to the task, use it directly — do not perform speculative file reads when KB provides the answer.

   Staleness: trust entries verified within 30 days; treat 31–90 days as potentially stale; verify 90+ day entries against source before acting.

4. **Work**: Perform the task

4. **Check children**: Before completing, verify no open children remain
   ```bash
   9p read agent/beads/children/bd-abc | jq 'any(.status != "closed")'
   # Must return false before proceeding
   ```

5. **Complete**: Atomically complete the bead (only after all children are closed)
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

## Hierarchical Completion Protocol

**Every agent must follow this protocol when completing any bead.** This applies universally — whether you are a worker bot, an orchestrator, or a standalone agent.

Before marking a bead complete, always verify its children are resolved:

```bash
# 1. Check for children
9p read agent/beads/children/bd-abc | jq .

# 2. If children exist, work through each one:
#    a. Claim it
echo "claim bd-abc.1 $AGENT_ID" | 9p write agent/beads/ctl
#    b. Do the work (recurse: check bd-abc.1's children too)
#    c. Complete it
echo "complete bd-abc.1" | 9p write agent/beads/ctl

# 3. Verify ALL children closed before proceeding
9p read agent/beads/children/bd-abc | jq 'all(.status == "closed")'
# Must output: true

# 4. Only then complete the parent
echo "complete bd-abc" | 9p write agent/beads/ctl
```

This protocol applies recursively. If a child has grandchildren, resolve them before completing the child.

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
**Instead**: Always `echo "claim bd-xxx $AGENT_ID" | 9p write agent/beads/ctl` before starting.

### ❌ NEVER complete a bead that has open children
**Why**: Completing a parent before its children are done orphans in-progress work and misrepresents task completion.
**Instead**: Check `9p read agent/beads/children/bd-abc | jq 'all(.status == "closed")'` and wait until it returns `true`.

## Quick Command Reference

### List ready work
```bash
9p read agent/beads/ready | jq
```

### Create bead
```bash
echo "new \"Implement feature\" \"Add new capability\"" | 9p write agent/beads/ctl
```

The server emits lint warnings to stderr if the description is missing quality signals (file path, function name/line, acceptance criterion, minimum length). Suppress with `--no-lint` or prefix the title with `IDEA:`:

```bash
# Bypass lint for idea-type or trivial beads
echo "new \"IDEA: Explore caching\" \"Some rough thoughts\" --no-lint" | 9p write agent/beads/ctl
```

### Create child bead
```bash
echo "new \"Design API\" \"Define interfaces\" bd-abc" | 9p write agent/beads/ctl
```

### Claim bead
```bash
echo "claim bd-abc $AGENT_ID" | 9p write agent/beads/ctl
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

### Query with JSON filter
```bash
# Find all open bugs assigned to you
echo '{"status":"open","issue_type":"bug","assignee":"agent-123"}' | 9p write agent/beads/query
9p read agent/beads/query | jq

# Find high priority tasks (priority 1-2)
echo '{"priority_min":1,"priority_max":2}' | 9p write agent/beads/query
9p read agent/beads/query | jq

# Find beads with specific label
echo '{"labels":["urgent"]}' | 9p write agent/beads/query
9p read agent/beads/query | jq

# Find unassigned open tasks
echo '{"status":"open","no_assignee":true}' | 9p write agent/beads/query
9p read agent/beads/query | jq
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
echo "claim bd-abc $AGENT_ID" | 9p write agent/beads/ctl

# Verify claim
9p read agent/beads/bd-abc/json | jq '.status, .assignee'

# Do the work...

# Complete the bead
echo "complete bd-abc" | 9p write agent/beads/ctl

# Verify completion
9p read agent/beads/bd-abc/json | jq .status
```
