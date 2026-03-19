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
tool: list_mounts.sh
```
Then find the entry matching your cwd and extract the mount name.

If no mount exists, create one (substitute actual path):
```
Tool: execute_code
tool: mount_beads.sh
args: ["--cwd", "/path/to/project"]
```

Initialize beads in a mount (creates .beads/):
```
Tool: execute_code
tool: init_beads.sh
args: ["--mount", "<mount>", "--prefix", "<prefix>"]
```

## Tools

All tools require `--mount <mount>`. Exceptions: `list_mounts.sh`, `mount_beads.sh`, `umount_beads.sh`.

**CRITICAL:** Always use `sandbox: default` with beads tools. The tools derive scope from cwd, which must match the project directory.

List mounts:
```
Tool: execute_code
tool: list_mounts.sh
sandbox: default
```

Wait for a bead on your mount (blocks until one is ready, returns full bead JSON including comments, then exits — bot decides whether to claim):
```
Tool: execute_code
tool: wait_for_bead.sh
args: ["--mount", "<mount>"]
sandbox: default
```

List ready beads (open, unblocked):
```
Tool: execute_code
tool: list_ready_beads.sh
args: ["--mount", "<mount>"]
sandbox: default
```

List all open beads (excludes closed):
```
Tool: execute_code
tool: list_beads.sh
args: ["--mount", "<mount>"]
sandbox: default
```

Search beads by id, title, or description (includes closed):
```
Tool: execute_code
tool: search_beads.sh
args: ["--mount", "<mount>", "--query", "<query>"]
```

Lookup bead by ID:
```
Tool: execute_code
tool: search_beads.sh
args: ["--mount", "<mount>", "--id", "<id>"]
```

**Note:** `list_beads.sh` and `list_ready_beads.sh` only return open beads. To find closed beads, use `search_beads.sh`.

Read bead JSON:
```
Tool: execute_code
tool: read_bead.sh
args: ["--mount", "<mount>", "--id", "<id>", "--property", "json"]
sandbox: default
```

Read bead comments:
```
Tool: execute_code
tool: read_bead_comments.sh
args: ["--mount", "<mount>", "--id", "<id>"]
sandbox: default
```

**IMPORTANT:** When working on a bead, always check `comment_count` in the JSON output. If > 0, read comments for additional context, decisions, or blockers.

Claim bead:
```
Tool: execute_code
tool: claim_bead.sh
args: ["--mount", "<mount>", "--id", "<id>"]
sandbox: default
```

Complete bead:
```
Tool: execute_code
tool: complete_bead.sh
args: ["--mount", "<mount>", "--id", "<id>"]
sandbox: default
```

Fail bead:
```
Tool: execute_code
tool: fail_bead.sh
args: ["--mount", "<mount>", "--id", "<id>", "--reason", "<reason>"]
sandbox: default
```

Create bead (starts deferred — use open_bead.sh when ready):
```
Tool: execute_code
tool: create_bead.sh
args: ["--mount", "<mount>", "--title", "<title>", "--desc", "<desc>", "--parent", "<id>"]
sandbox: default
```
`--desc`, `--parent`, `--no-lint`, `--capability low|standard|high` are all optional.

Update bead field:
```
Tool: execute_code
tool: update_bead.sh
args: ["--mount", "<mount>", "--id", "<id>", "--field", "<field>", "--value", "<value>"]
sandbox: default
```
Valid fields: `title`, `description`, `design`, `acceptance_criteria`, `notes`, `issue_type`, `estimated_minutes`, `external_ref`, `spec_id`, `priority`, `due_at`, `work_type`. Status and assignee are managed by atomic operations only.

Open (promote deferred to ready):
```
Tool: execute_code
tool: open_bead.sh
args: ["--mount", "<mount>", "--id", "<id>"]
sandbox: default
```

Defer bead:
```
Tool: execute_code
tool: defer_bead.sh
args: ["--mount", "<mount>", "--id", "<id>"]
sandbox: default
```
Optional: append `"--until", "<RFC3339-time>"` to defer until a specific time.

Reopen closed bead:
```
Tool: execute_code
tool: reopen_bead.sh
args: ["--mount", "<mount>", "--id", "<id>"]
sandbox: default
```

Relate two beads (soft link, no blocking):
```
Tool: execute_code
tool: relate_beads.sh
args: ["--mount", "<mount>", "--bead1", "<id-1>", "--bead2", "<id-2>"]
sandbox: default
```

Add dependency (child blocked by parent):
```
Tool: execute_code
tool: add_dependency.sh
args: ["--mount", "<mount>", "--child", "<child>", "--parent", "<parent>"]
sandbox: default
```

Comment on bead:
```
Tool: execute_code
tool: comment_bead.sh
args: ["--mount", "<mount>", "--id", "<id>", "--text", "<text>"]
sandbox: default
```

Label bead:
```
Tool: execute_code
tool: label_bead.sh
args: ["--mount", "<mount>", "--id", "<id>", "--label", "<label>"]
sandbox: default
```

Delete bead:
```
Tool: execute_code
tool: delete_bead.sh
args: ["--mount", "<mount>", "--id", "<id>"]
sandbox: default
```

**Note:** Delete does not cascade. To delete a parent and all children, delete children first, then parent.

Unmount project:
```
Tool: execute_code
tool: umount_beads.sh
args: ["--mount", "<mount>"]
sandbox: default
```
**Note:** Unmounting is an emergency stop. The orphan recovery cron will not spawn agents for unmounted projects. Remount to resume recovery.

Sync project:
```
Tool: execute_code
tool: sync_beads.sh
args: ["--mount", "<mount>"]
sandbox: default
```

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
	- Good: `complete_bead.sh --mount myproj --id bd-123`
	- Bad: `update_bead.sh --mount myproj --id bd-123 --field status --value closed`

## Description Quality

- The goal is to enable cold completion of the task, without additional research or code exploration
- Imperative verb + file path + `` `funcName()` `` or Acme file address (`/path/to/file:123,125` not `/path/to/file:123-125`)
- "How" signal (following X, same as Y) + acceptance criterion (must/should/returns)
- Backticked identifiers + cross-ref for >150 chars

Bypass lint: title has `IDEA:` prefix.
