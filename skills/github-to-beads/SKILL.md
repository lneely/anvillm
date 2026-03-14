---
name: github-to-beads
intent: tasks, github
description: Import GitHub issues and PRs into beads task tracking. Use when user asks to pull GitHub issue info, define work from an issue, or create tasks from GitHub.
---

# GitHub to Beads

Import GitHub issues into beads.

## Usage

```
Tool: execute_code
tool: github_to_beads.sh
args: ["--mount", "<mount>", "--repo", "<owner/repo>", "--issue", "<issue_number>"]
```

**What it does:**
1. Fetches issue data via `gh` CLI
2. Creates bead: `#NUMBER: title` → title, body → description
3. Maps labels to issue_type (bug label → bug type)
4. Sets status (closed issues → closed status)
5. Parses task list (`- [ ]` items) as child beads
6. Skips if already imported

## When to Use

- User asks to "pull" or "import" a GitHub issue
- User wants to define work based on a GitHub issue
- User asks to create beads/tasks from GitHub

## When NOT to Use

- Just viewing issue info (use `gh issue view`)
- Modifying GitHub issues (this is pull-only)
- General bead management (use `beads` skill)

## Manual GitHub Commands

```bash
gh issue view 123 --repo owner/repo                    # human readable
gh issue view 123 --repo owner/repo --json number,title,body  # JSON
```

## After Import

Review and adjust:
- Remove beads bot doesn't need
- Refine descriptions
- Add context
