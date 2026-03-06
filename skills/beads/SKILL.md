---
name: beads
intent: tasks, workflow
description: Manage tasks using the beads 9P interface. Use when creating, updating, listing, or deleting tasks/beads, or managing task dependencies.
---

# Beads Skill

## Project Mount Discovery

Find your project's beads mount:
```
Tool: execute_code
sandbox: anvilmcp
code: MOUNT=$(bash <(9p read agent/tools/list_mounts.sh) | grep "$(pwd)" | awk '{print $1}'); echo $MOUNT
```

If no mount exists, create one:
```
Tool: execute_code
sandbox: anvilmcp
code: bash <(9p read agent/tools/mount_beads.sh) "$(pwd)"
```

## Tools

All tools require `<mount>` as first argument. Exceptions: `list_mounts.sh`, `mount_beads.sh`, `umount_beads.sh`.

List mounts:
```
Tool: execute_code
sandbox: anvilmcp
code: bash <(9p read agent/tools/list_mounts.sh)
```

List ready beads:
```
Tool: execute_code
sandbox: anvilmcp
code: bash <(9p read agent/tools/list_ready_beads.sh) <mount>
```

List all beads:
```
Tool: execute_code
sandbox: anvilmcp
code: bash <(9p read agent/tools/list_beads.sh) <mount>
```

Read bead JSON:
```
Tool: execute_code
sandbox: anvilmcp
code: bash <(9p read agent/tools/read_bead.sh) <mount> <id> json
```

Read bead comments:
```
Tool: execute_code
sandbox: anvilmcp
code: bash <(9p read agent/tools/read_bead.sh) <mount> <id> comments
```

**IMPORTANT:** When working on a bead, always check `comment_count` in the JSON output. If > 0, read comments for additional context, decisions, or blockers.

Claim bead:
```
Tool: execute_code
sandbox: anvilmcp
code: bash <(9p read agent/tools/claim_bead.sh) <mount> <id> [$AGENT_ID]
```

Complete bead:
```
Tool: execute_code
sandbox: anvilmcp
code: bash <(9p read agent/tools/complete_bead.sh) <mount> <id>
```

Fail bead:
```
Tool: execute_code
sandbox: anvilmcp
code: bash <(9p read agent/tools/fail_bead.sh) <mount> <id> "reason"
```

Create bead:
```
Tool: execute_code
sandbox: anvilmcp
code: bash <(9p read agent/tools/create_bead.sh) <mount> "title" "desc" [parent]
```

Update bead:
```
Tool: execute_code
sandbox: anvilmcp
code: bash <(9p read agent/tools/update_bead.sh) <mount> <id> <field> <value>
```

Add dependency:
```
Tool: execute_code
sandbox: anvilmcp
code: bash <(9p read agent/tools/add_dependency.sh) <mount> <child> <parent>
```

Comment on bead:
```
Tool: execute_code
sandbox: anvilmcp
code: bash <(9p read agent/tools/comment_bead.sh) <mount> <id> "text"
```

Label bead:
```
Tool: execute_code
sandbox: anvilmcp
code: bash <(9p read agent/tools/label_bead.sh) <mount> <id> <label>
```

Delete bead:
```
Tool: execute_code
sandbox: anvilmcp
code: bash <(9p read agent/tools/delete_bead.sh) <mount> <id>
```

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
