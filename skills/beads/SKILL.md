---
name: beads
intent: tasks, workflow
description: Manage tasks using the beads 9P interface. Use when creating, updating, listing, or deleting tasks/beads, or managing task dependencies.
---

# Beads Skill

## Project Mount Discovery

1. Find your project's beads mount: `MOUNT=$(bash <(9p read agent/tools/beads/list_mounts.sh) | grep "$(pwd)" | awk '{print $1}')`
2. If no mount exists, create one and use it: `bash <(9p read agent/tools/beads/mount_beads.sh) "$(pwd)"`. The mount will be named for the base directory of $(pwd).

## Tools

**IMPORTANT:** All beads tools listed must be run via `execute_code` tool.

**IMPORTANT:** All tools require `<mount>` as first argument. Exceptions: `list_mounts.sh`, `mount_beads.sh`, `umount_beads.sh`.

- List mounts: `bash <(9p read tools/beads/list_mounts.sh)`
- List ready beads: `bash <(9p read tools/beads/list_ready_beads.sh) <mount>`
- List all beads: `bash <(9p read tools/beads/list_beads.sh) <mount>`
- Read bead: `bash <(9p read tools/beads/read_bead.sh) <mount> <id> <property>`
- Read bead JSON: `bash <(9p read tools/beads/read_bead.sh) <mount> <id> json`
- Read bead comments: `bash <(9p read tools/beads/read_bead.sh) <mount> <id> comments`

**IMPORTANT:** When working on a bead, always check `comment_count` in the JSON output. If > 0, read comments for additional context, decisions, or blockers.

- Claim bead: `bash <(9p read tools/beads/claim_bead.sh) <mount> bd-abc [$AGENT_ID]`
- Complete bead: `bash <(9p read tools/beads/complete_bead.sh) <mount> bd-abc`
- Fail bead: `bash <(9p read tools/beads/fail_bead.sh) <mount> bd-abc "reason"`
- Create bead: `bash <(9p read tools/beads/create_bead.sh) <mount> "title" "desc" [parent]`
- Update bead: `bash <(9p read tools/beads/update_bead.sh) <mount> bd-abc <field> <value>`
- Add dependency: `bash <(9p read tools/beads/add_dependency.sh) <mount> <child> <parent>`
- Comment on bead: `bash <(9p read tools/beads/comment_bead.sh) <mount> bd-abc "text"`
- Label bead: `bash <(9p read tools/beads/label_bead.sh) <mount> bd-abc <label>`
- Delete bead: `bash <(9p read tools/beads/delete_bead.sh) <mount> bd-abc`

**Note:** Delete does not cascade. To delete a parent and all children, delete children first, then parent.

## Rules

- Discover your mount first using mtab
- Claim before working
- Complete children before parent, if present
- Only parse JSON output
- Never access `.beads/` directly
- When creating, updating, or commenting on a bead, always escape JSON properly for bash
- Comment during work (decisions, dead ends, discoveries)
- Search past beads before starting
- Always favor atomic actions
	- Good: complete_bead myproj bd-123
	- Bad: update_bead myproj bd-123 status closed

## Description Quality

- The goal is to enable cold completion of the task, without additional research or code exploration
- Imperative verb + file path + `` `funcName()` `` or Acme file address (`/path/to/file:123,125` not `/path/to/file:123-125`)
- "How" signal (following X, same as Y) + acceptance criterion (must/should/returns)
- Backticked identifiers + cross-ref for >150 chars

Bypass lint: title has `IDEA:` prefix.
