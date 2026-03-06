---
name: jira-to-beads
intent: tasks, jira
description: Import Jira tickets into beads task tracking. Use when user asks to pull Jira ticket info, define work from a ticket, or create tasks from an epic/story.
---

# Jira to Beads

Import Jira ticket hierarchies into beads.

## Usage

```
Tool: execute_code
sandbox: anvilmcp
code: bash <(9p read agent/tools/jira/jira_to_beads.sh) PROJ-12345
```

**What it does:**
1. Finds root ticket (walks up parent chain)
2. Creates beads top-down (parent → children)
3. Maps fields: `KEY: summary` → title, description → description
4. Recursively imports entire hierarchy
5. Skips if already imported

## When to Use

- User asks to "pull" or "import" a Jira ticket
- User wants to define work based on a Jira epic/story
- User asks to create beads/tasks from Jira

## When NOT to Use

- Just viewing ticket info (use `jira issue view`)
- Modifying Jira tickets (this is pull-only)
- General bead management (use `beads` skill)

## Manual Jira Commands

```bash
jira issue view PROJ-12345 --plain          # human readable
jira issue view PROJ-12345 --raw            # JSON
jira issue list --parent PROJ-12345         # list children
```

## After Import

Review and adjust:
- Remove beads bot doesn't need
- Refine descriptions
- Add context
