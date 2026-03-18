# Beads Integration

9P filesystem integration for [steveyegge/beads](https://github.com/steveyegge/beads) task tracking.

## Overview

Beads provides persistent, structured task memory for coding agents. Tasks persist across crashes, enabling agents to resume work and coordinate through dependency graphs.

**Storage:** Dolt (version-controlled SQL database) provides MVCC, ACID transactions, cell-level diffs, and JSONL export for git portability.

## Filesystem Structure

```
anvillm/beads/
├── ctl                    # Control file for commands
├── query                  # Filtered query endpoint (write JSON filter, read results)
├── list                   # All beads (JSON)
├── ready                  # Ready beads (no blockers, JSON)
├── pending                # Beads awaiting human approval/review (JSON)
├── stats                  # Statistics (JSON)
├── blocked                # Blocked issues (JSON)
├── stale                  # Stale issues (not updated in 30+ days, JSON)
├── config                 # All configuration (JSON)
├── search/<query>         # Text search results (JSON)
├── by-ref/<ref>           # Issue by external reference (JSON)
├── batch/<id1,id2,...>    # Batch lookup by IDs (JSON)
├── label/<label>          # Issues with label (JSON)
├── children/<id>          # Direct children of parent (JSON)
└── <bead-id>/
    ├── status             # open | in_progress | closed
    ├── title              # Bead title
    ├── description        # Bead description
    ├── assignee           # Assigned actor
    ├── json               # Full bead with blockers (JSON)
    ├── comments           # Issue comments (JSON)
    ├── labels             # Issue labels (JSON)
    ├── dependents         # Issues that depend on this (JSON)
    ├── dependencies-meta  # Dependencies with metadata (JSON)
    ├── dependents-meta    # Dependents with metadata (JSON)
    ├── tree               # Dependency tree (JSON)
    └── events             # Event history (JSON)
```

## Control Commands

| Command | Format | Description |
|---------|--------|-------------|
| `new` | `new 'title' ['description'] [parent-id]` | Create new bead (optionally as child) |
| `create` | `create 'title' ['description'] [parent-id]` | Alias for new |
| `claim` | `claim <bead-id>` | Claim bead (sets assignee + in_progress) |
| `complete` | `complete <bead-id>` | Mark bead as completed |
| `close` | `close <bead-id>` | Alias for complete |
| `fail` | `fail <bead-id> 'reason'` | Mark bead as failed |
| `dep` | `dep <child-id> <parent-id>` | Add dependency (parent blocks child) |
| `add-dep` | `add-dep <child-id> <parent-id>` | Alias for dep |
| `undep` | `undep <child-id> <parent-id>` | Remove dependency |
| `rm-dep` | `rm-dep <child-id> <parent-id>` | Alias for undep |
| `update` | `update <bead-id> <field> 'value'` | Update bead field |
| `delete` | `delete <bead-id>` | Delete bead |
| `rm` | `rm <bead-id>` | Alias for delete |
| `comment` | `comment <bead-id> 'text'` | Add comment to bead |
| `label` | `label <bead-id> 'label'` | Add label to bead |
| `unlabel` | `unlabel <bead-id> 'label'` | Remove label from bead |
| `set-capability` | `set-capability <bead-id> low\|standard\|high` | Set capability level (replaces existing) |
| `init` | `init [prefix]` | Initialize beads with custom ID prefix (default: bd) |
| `batch-create` | `batch-create <json-array>` | Create multiple beads from JSON array |

## Usage Examples

```sh
# Create bead
echo "new 'Implement auth' 'Add OAuth support'" | 9p write anvillm/beads/ctl

# Claim bead
echo "claim bd-a1b2" | 9p write anvillm/beads/ctl

# Complete bead
echo "complete bd-a1b2" | 9p write anvillm/beads/ctl

# List ready beads
9p read anvillm/beads/ready

# List blocked beads
9p read anvillm/beads/blocked

# List stale beads (not updated in 30+ days)
9p read anvillm/beads/stale

# Get statistics
9p read anvillm/beads/stats

# Get all configuration
9p read anvillm/beads/config

# Search for beads
9p read anvillm/beads/search/authentication

# Get bead by external reference
9p read anvillm/beads/by-ref/JIRA-123

# Batch lookup multiple beads
9p read anvillm/beads/batch/bd-a1b2,bd-c3d4,bd-e5f6

# Get beads with specific label
9p read anvillm/beads/label/backend

# Get children of a parent bead
9p read anvillm/beads/children/bd-a1b2

# Read bead status
9p read anvillm/beads/bd-a1b2/status

# Read full bead with blockers
9p read anvillm/beads/bd-a1b2/json

# Read bead comments
9p read anvillm/beads/bd-a1b2/comments

# Read bead labels
9p read anvillm/beads/bd-a1b2/labels

# Read bead dependents
9p read anvillm/beads/bd-a1b2/dependents

# Read dependencies with metadata
9p read anvillm/beads/bd-a1b2/dependencies-meta

# Read dependents with metadata
9p read anvillm/beads/bd-a1b2/dependents-meta

# Read dependency tree
9p read anvillm/beads/bd-a1b2/tree

# Read event history
9p read anvillm/beads/bd-a1b2/events

# Add dependency (parent blocks child)
echo "dep bd-child bd-parent" | 9p write anvillm/beads/ctl

# Update bead field
echo "update bd-a1b2 priority 1" | 9p write anvillm/beads/ctl

# Add comment
echo "comment bd-a1b2 'Work in progress'" | 9p write anvillm/beads/ctl

# Add label
echo "label bd-a1b2 'backend'" | 9p write anvillm/beads/ctl

# Set capability level
echo "set-capability bd-a1b2 high" | 9p write anvillm/beads/ctl

# Initialize with custom prefix
echo "init myprefix" | 9p write anvillm/beads/ctl
```

## Filtered Queries

The `anvillm/beads/query` endpoint accepts JSON filter criteria for complex queries.

**Note:** The query endpoint is stateful per session. Write a filter to set the query, then read to retrieve results. The filter persists until overwritten.

```sh
# Query by assignee and priority
echo '{"assignee":"alice","priority":1}' | 9p write anvillm/beads/query
9p read anvillm/beads/query

# Query by status and type
echo '{"status":"open","issue_type":"bug"}' | 9p write anvillm/beads/query
9p read anvillm/beads/query

# Query by labels (all must match)
echo '{"labels":["backend","urgent"]}' | 9p write anvillm/beads/query
9p read anvillm/beads/query

# Query by parent ID
echo '{"parent_id":"bd-abc"}' | 9p write anvillm/beads/query
9p read anvillm/beads/query
```

Available filter fields:
- `assignee` (string): Filter by assignee
- `status` (string): Filter by status (open, in_progress, closed, etc.)
- `issue_type` (string): Filter by type (task, bug, feature, etc.)
- `priority` (int): Filter by priority (1-5)
- `labels` (array): Filter by labels (all must match)
- `parent_id` (string): Filter by parent ID

## Initialization

Initialize beads in a project (creates `.beads/` directory with Dolt database):

```go
import "anvillm/internal/beads"

err := beads.InitBeads("/path/to/project")
```

Agents access via 9P at `anvillm/beads/`:

```sh
cat anvillm/beads/ready
echo 'claim bd-xyz' > anvillm/beads/ctl
```

## MCP Integration

Beads are accessed via `execute_code` through 9P:

```bash
# Create a bead
echo 'new "Implement login" "Add JWT auth"' | 9p write anvillm/beads/ctl

# List beads
9p read anvillm/beads/list | jq '.[] | "\(.id): \(.title)"'

# Claim and complete
echo 'claim bd-a1b2' | 9p write anvillm/beads/ctl
echo 'complete bd-a1b2' | 9p write anvillm/beads/ctl
```

## Benefits

- **Crash resilience** — Beads persist in Dolt database
- **Resumability** — Agents pick up where others left off
- **Dependency tracking** — Automatic blocking relationships
- **Version control** — Full history via Dolt
- **Git portability** — JSONL export syncs via git
- **Scriptability** — Standard file operations
- **Pure Go** — No Python dependency

**Implementation:** `beads.go` (storage wrapper), `fs.go` (9P handlers) — ~300 LOC

## See Also

- [steveyegge/beads](https://github.com/steveyegge/beads)
- [Dolt](https://github.com/dolthub/dolt)
