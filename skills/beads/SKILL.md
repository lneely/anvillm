---
name: beads
intent: tasks, workflow
description: Manage tasks using the beads 9P interface. Use when creating, updating, listing, or deleting tasks/beads, or managing task dependencies.
---

# Beads Task System

Hierarchical task tracking via 9P. Persists to Dolt immediately.

## Project Mount Discovery

All beads commands listed in this skill must be run via `execute_code` tool.

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

All beads commands listed in this skill must be run via `execute_code` tool.

**CRITICAL:** All commands require `<mount>` as first argument. Exceptions: `list_mounts.sh`, `mount_beads.sh`, `umount_beads.sh`.

- Claim bead: `claim_bead.sh <mount> bd-abc [$AGENT_ID]`
- Complete bead: `complete_bead.sh <mount> bd-abc`
- Fail bead: `fail_bead.sh <mount> bd-abc "reason"`
- Create bead: `create_bead.sh <mount> "title" "desc" [parent]`
- Update bead: `update_bead.sh <mount> bd-abc <field> <value>`
- Add dependency: `add_dependency.sh <mount> <child> <parent>`
- Comment on bead: `comment_bead.sh <mount> bd-abc "text"`
- Label bead: `label_bead.sh <mount> bd-abc <label>`
- Delete bead: `delete_bead.sh <mount> bd-abc`

**Note:** Delete does not cascade. To delete a parent and all children, delete children first, then parent.

## Queries

- List mounts: `list_mounts.sh`
- List ready beads: `list_ready_beads.sh <mount>`
- List all beads: `list_beads.sh <mount>`
- Read bead: `read_bead.sh <mount> <id> <property>`
- Read bead JSON: `read_bead.sh <mount> <id> json`

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
