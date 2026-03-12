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
code: MOUNT=$(bash <(9p read anvillm/tools/list_mounts.sh) | grep "$(pwd)" | awk '{print $1}'); echo $MOUNT
```

If no mount exists, create one (substitute actual path):
```
Tool: execute_code
tool: mount_beads.sh
args: ["/path/to/project"]
```

Initialize beads in a mount (creates .beads/):
```
Tool: execute_code
tool: init_beads.sh
args: ["<mount>", "<prefix>"]
```

## Tools

All tools require `<mount>` as first argument. Exceptions: `list_mounts.sh`, `mount_beads.sh`, `umount_beads.sh`.

List mounts:
```
Tool: execute_code
tool: list_mounts.sh
```

List ready beads (open, unblocked):
```
Tool: execute_code
tool: list_ready_beads.sh
args: ["<mount>"]
```

List all open beads (excludes closed):
```
Tool: execute_code
tool: list_beads.sh
args: ["<mount>"]
```

Search beads by id, title, or description (includes closed):
```
Tool: execute_code
tool: search_beads.sh
args: ["<mount>", "<query>"]
```

**Note:** `list_beads.sh` and `list_ready_beads.sh` only return open beads. To find closed beads, use `search_beads.sh`.

Read bead JSON:
```
Tool: execute_code
tool: read_bead.sh
args: ["<mount>", "<id>", "json"]
```

Read bead comments:
```
Tool: execute_code
tool: read_bead_comments.sh
args: ["<mount>", "<id>"]
```

**IMPORTANT:** When working on a bead, always check `comment_count` in the JSON output. If > 0, read comments for additional context, decisions, or blockers.

Claim bead:
```
Tool: execute_code
tool: claim_bead.sh
args: ["<mount>", "<id>", "$AGENT_ID"]
```

Complete bead:
```
Tool: execute_code
tool: complete_bead.sh
args: ["<mount>", "<id>"]
```

Fail bead:
```
Tool: execute_code
tool: fail_bead.sh
args: ["<mount>", "<id>", "<reason>"]
```

Create bead:
```
Tool: execute_code
tool: create_bead.sh
args: ["<mount>", "<title>", "<desc>", "<parent>"]
```

Update bead:
```
Tool: execute_code
tool: update_bead.sh
args: ["<mount>", "<id>", "<field>", "<value>"]
```

Add dependency:
```
Tool: execute_code
tool: add_dependency.sh
args: ["<mount>", "<child>", "<parent>"]
```

Comment on bead:
```
Tool: execute_code
tool: comment_bead.sh
args: ["<mount>", "<id>", "<text>"]
```

Label bead:
```
Tool: execute_code
tool: label_bead.sh
args: ["<mount>", "<id>", "<label>"]
```

Delete bead:
```
Tool: execute_code
tool: delete_bead.sh
args: ["<mount>", "<id>"]
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
