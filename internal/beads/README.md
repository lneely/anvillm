# Beads Integration

9P filesystem integration for [steveyegge/beads](https://github.com/steveyegge/beads) task tracking.

## Overview

Beads provides persistent, structured memory for coding agents. This integration exposes beads through the 9P filesystem, allowing agents to create, claim, and complete tasks using simple file operations.

## Storage Backend

Uses Dolt (version-controlled SQL database) via the beads library:
- **MVCC**: Built-in multi-version concurrency control
- **Version control**: Cell-level diffs and merges
- **Multi-writer**: Server mode supports concurrent agents
- **ACID**: Full transaction guarantees
- **Git integration**: JSONL export for portability

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

## Usage

### Create a bead
```sh
echo "new 'Implement auth' dev 'Add OAuth support'" | 9p write agent/beads/ctl
```

### Claim a bead
```sh
echo "claim bd-a1b2" | 9p write agent/beads/ctl
```

### Complete a bead
```sh
echo "complete bd-a1b2" | 9p write agent/beads/ctl
```

### List ready beads
```sh
9p read agent/beads/ready
```

### Read bead status
```sh
9p read agent/beads/bd-a1b2/status
```

### Add dependency
```sh
echo "dep bd-child bd-parent" | 9p write agent/beads/ctl
```

## Control Commands

| Command | Format | Description |
|---------|--------|-------------|
| `new` | `new 'title' role ['description']` | Create new bead |
| `claim` | `claim <bead-id>` | Claim bead for current actor |
| `complete` | `complete <bead-id>` | Mark bead as completed |
| `fail` | `fail <bead-id> 'reason'` | Mark bead as failed |
| `dep` | `dep <child-id> <parent-id>` | Add dependency (parent blocks child) |

## Initialization

Initialize beads in a project:

```go
import "anvillm/internal/beads"

err := beads.InitBeads("/path/to/project")
```

This creates `.beads/` directory with Dolt database.

## Integration with anvilsrv

The beads filesystem is mounted at `agent/beads/` in the 9P server. Agents access it like any other file:

```
# From agent session
cat agent/beads/ready
echo 'claim bd-xyz' > agent/beads/ctl
```

## MCP Integration (Optional)

anvilmcp can expose beads operations as MCP tools:

```json
{
  "name": "create_bead",
  "description": "Create a new task bead",
  "inputSchema": {
    "type": "object",
    "properties": {
      "title": {"type": "string"},
      "role": {"type": "string"},
      "description": {"type": "string"}
    }
  }
}
```

Implementation: MCP tool calls `9p write agent/beads/ctl`.

## Benefits

1. **Crash resilience**: Beads persist in Dolt database
2. **Resumability**: Agents can pick up where others left off
3. **Dependency tracking**: Automatic blocking relationships
4. **Version control**: Full audit trail via Dolt
5. **Git portability**: JSONL export syncs via git
6. **Scriptability**: Standard file operations
7. **No Python**: Pure Go implementation

## Implementation

- `beads.go`: Storage wrapper around steveyegge/beads
- `fs.go`: 9P filesystem handlers
- ~300 LOC total

## See Also

- [steveyegge/beads](https://github.com/steveyegge/beads)
- [Dolt](https://github.com/dolthub/dolt)
- [IDEAS.md](../../IDEAS.md) - Original beads concept
