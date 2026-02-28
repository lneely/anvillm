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
MOUNT=$(bash <(9p read agent/tools/beads/list_mounts.sh) | grep "$MY_CWD" | awk '{print $1}')
```

If no mount exists, create one:
```bash
bash <(9p read agent/tools/beads/mount_beads.sh) "$MY_CWD"
```

## Commands

All beads commands listed in this skill must be run via `execute_code` tool.

**CRITICAL:** All commands require `<mount>` as first argument (discovered via list_mounts.sh), except `mount_beads.sh` and `umount_beads.sh`.

- Claim bead: `bash <(9p read agent/tools/beads/claim_bead.sh) <mount> bd-abc [$AGENT_ID]`
- Complete bead: `bash <(9p read agent/tools/beads/complete_bead.sh) <mount> bd-abc`
- Fail bead: `bash <(9p read agent/tools/beads/fail_bead.sh) <mount> bd-abc "reason"`
- Create bead: `bash <(9p read agent/tools/beads/create_bead.sh) <mount> "title" "desc" [parent]`
- Update bead: `bash <(9p read agent/tools/beads/update_bead.sh) <mount> bd-abc <field> <value>`
- Add dependency: `bash <(9p read agent/tools/beads/add_dependency.sh) <mount> <child> <parent>`
- Comment on bead: `bash <(9p read agent/tools/beads/comment_bead.sh) <mount> bd-abc "text"`
- Label bead: `bash <(9p read agent/tools/beads/label_bead.sh) <mount> bd-abc <label>`
- Delete bead: `bash <(9p read agent/tools/beads/delete_bead.sh) <mount> bd-abc`

**Note:** Delete does not cascade. To delete a parent and all children, delete children first, then parent.

## Queries

- List ready beads: `bash <(9p read agent/tools/beads/list_ready_beads.sh) <mount>`
- Read bead: `bash <(9p read agent/tools/beads/read_bead.sh) <mount> <id> json`
- List children: `bash <(9p read agent/tools/beads/read_bead.sh) <mount> <id> children`
- Search beads: `bash <(9p read agent/tools/beads/read_bead.sh) <mount> search <term>`
- View history: `bash <(9p read agent/tools/beads/read_bead.sh) <mount> <id> events`

## Workflow

1. Find a ready task
2. Claim the ticket being worked on
3. Use agent-kb skill to search the knowledge base.
4. Do work
5. Verify all children completed
6. Only if all children completed, complete the parent. Otherwise, work on other unblocked child tasks.

## Rules

- Discover your mount first using list_mounts.sh
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
