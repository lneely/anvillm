---
name: beads
intent: tasks, workflow
description: Manage tasks using the beads 9P interface. Use when creating, updating, listing, or deleting tasks/beads, or managing task dependencies.
---

# Beads Task System

Hierarchical task tracking via 9P. Persists to Dolt immediately.

## Project Mount Discovery

Find your project's beads mount:
```bash
MY_CWD=$(pwd)
MOUNT=$(list_mounts.sh | grep "$MY_CWD" | awk '{print $1}')
```

If no mount exists, create one:
```bash
mount_beads.sh "$MY_CWD"
```

## Commands

- `claim_bead.sh bd-abc [$AGENT_ID]` - claim and start
- `complete_bead.sh bd-abc` - finish
- `fail_bead.sh bd-abc "reason"` - fail with reason
- `create_bead.sh "title" "desc" [parent]` - create (child if parent given)
- `update_bead.sh bd-abc <field> <value>` - modify field
- `add_dependency.sh <child> <parent>` - add blocker
- `comment_bead.sh bd-abc "text"` - leave context
- `label_bead.sh bd-abc <label>` - tag
- `delete_bead.sh bd-abc` - delete (does NOT cascade to children)

**Note:** Delete does not cascade. To delete a parent and all children, delete children first, then parent.

## Queries

- `list_ready_beads.sh` - unblocked work
- `read_bead.sh <id> json` - single bead
- `read_bead.sh children <id>` - child beads
- `read_bead.sh search <term>` - search
- `read_bead.sh <id> events` - history

## Workflow

1. Find a ready task
2. Claim the ticket being worked on
3. Use agent-kb skill to search the knowledge base.
4. Do work
5. Verify all children completed
6. Only if all children completed, complete the parent. Otherwise, work on other unblocked child tasks.

## Rules

- Discover your mount first using mtab
- Claim before working
- Complete children before parent
- Only parse JSON output
- Never access `.beads/` directly
- Comment during work (decisions, dead ends, discoveries)
- Search past beads before starting

## Description Quality

Enable cold completion. Include:
- Imperative verb + file path + `` `funcName()` `` or Acme address (`file:123,125` not `file:123-125`)
- "How" signal (following X, same as Y) + acceptance criterion (must/should/returns)
- Backticked identifiers + cross-ref for >150 chars

Bypass lint: `IDEA:` prefix.
