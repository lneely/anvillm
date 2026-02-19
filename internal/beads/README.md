# Beads Integration

9P filesystem integration for [steveyegge/beads](https://github.com/steveyegge/beads) task tracking.

## Overview

Beads provides persistent, structured task memory for coding agents. Tasks persist across crashes, enabling agents to resume work and coordinate through dependency graphs.

**Storage:** Dolt (version-controlled SQL database) provides MVCC, ACID transactions, cell-level diffs, and JSONL export for git portability.

## Filesystem Structure

```
agent/beads/
├── ctl              # Control file for commands
├── list             # All beads (JSON)
├── ready            # Ready beads (no blockers, JSON)
└── <bead-id>/
    ├── status       # open | in_progress | closed
    ├── title        # Bead title
    ├── description  # Bead description
    ├── role         # Required role (dev, researcher, etc.)
    ├── assignee     # Assigned actor
    └── json         # Full bead as JSON
```

## Control Commands

| Command | Format | Description |
|---------|--------|-------------|
| `new` | `new 'title' role ['description']` | Create new bead |
| `claim` | `claim <bead-id>` | Claim bead for current actor |
| `complete` | `complete <bead-id>` | Mark bead as completed |
| `fail` | `fail <bead-id> 'reason'` | Mark bead as failed |
| `dep` | `dep <child-id> <parent-id>` | Add dependency (parent blocks child) |

## Usage Examples

```sh
# Create bead
echo "new 'Implement auth' dev 'Add OAuth support'" | 9p write agent/beads/ctl

# Claim bead
echo "claim bd-a1b2" | 9p write agent/beads/ctl

# Complete bead
echo "complete bd-a1b2" | 9p write agent/beads/ctl

# List ready beads
9p read agent/beads/ready

# Read bead status
9p read agent/beads/bd-a1b2/status

# Add dependency (parent blocks child)
echo "dep bd-child bd-parent" | 9p write agent/beads/ctl
```

## Initialization

Initialize beads in a project (creates `.beads/` directory with Dolt database):

```go
import "anvillm/internal/beads"

err := beads.InitBeads("/path/to/project")
```

Agents access via 9P at `agent/beads/`:

```sh
cat agent/beads/ready
echo 'claim bd-xyz' > agent/beads/ctl
```

## MCP Integration

anvilmcp exposes beads operations as MCP tools (calls `9p write agent/beads/ctl`):

```json
{
  "name": "create_bead",
  "inputSchema": {
    "properties": {
      "title": {"type": "string"},
      "role": {"type": "string"},
      "description": {"type": "string"}
    }
  }
}
```

## Benefits

- **Crash resilience** — Beads persist in Dolt database
- **Resumability** — Agents pick up where others left off
- **Dependency tracking** — Automatic blocking relationships
- **Version control** — Full audit trail via Dolt
- **Git portability** — JSONL export syncs via git
- **Scriptability** — Standard file operations
- **Pure Go** — No Python dependency

**Implementation:** `beads.go` (storage wrapper), `fs.go` (9P handlers) — ~300 LOC

## See Also

- [steveyegge/beads](https://github.com/steveyegge/beads)
- [Dolt](https://github.com/dolthub/dolt)
