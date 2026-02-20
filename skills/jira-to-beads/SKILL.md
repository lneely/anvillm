---
name: jira-to-beads
description: Import Jira tickets into beads task tracking. Use when user asks to pull Jira ticket info, define work from a ticket, or create tasks from an epic/story.
---

# Jira to Beads

## Purpose

Import Jira ticket hierarchies into beads, creating a task tree for agent work tracking.

## When to Use

- User asks to "pull" or "import" a Jira ticket
- User wants to define work based on a Jira epic or story
- User asks to create beads/tasks from Jira

## When NOT to Use

- User just wants to view ticket info (use `jira issue view` directly)
- User wants to modify Jira tickets (this is pull-only)
- General bead management (use `beads` skill)

## Jira CLI

Use the `jira` CLI (not `atlassian-cli`):

```bash
# View ticket (human readable)
jira issue view PROJ-12345 --plain

# View ticket (JSON for parsing)
jira issue view PROJ-12345 --raw

# List child tickets
jira issue list --parent PROJ-12345 --plain --no-truncate
```

## Import Process

### 1. Check Existing Beads

```bash
9p read agent/beads/list | grep "PROJ-12345"
```
Skip if already tracked.

### 2. Find Root Ticket

```bash
jira issue view PROJ-12345 --raw | jq '.fields.parent.key // empty'
```
Recursively fetch ancestors until no parent. Build tree top-down from root.

### 3. Build Task Tree

```bash
jira issue list --parent PROJ-12345 --plain --no-truncate
```
Recursively fetch children until reaching leaves.

### 4. Create Beads Top-Down

Create parent first, then children with parent ID. See `beads` skill for commands.

**CRITICAL**: Include Jira key in title: `PROJ-XXXXX: <summary>`

## Field Mapping

| Jira Field | Bead Field | Notes |
|------------|------------|-------|
| key + summary | title | Format: `PROJ-XXXXX: summary` |
| description | description | **MUST include ticket body** (truncate if very long) |
| parent ticket | parent_id | Pass parent bead ID |
| - | status | Always `open` initially |

**CRITICAL**: The bead description MUST contain the Jira ticket body/description. This provides essential context for working on the task. Extract plain text from the Jira ADF format.

## Including Comments and Linked Pages

### Comments
Include significant information from ticket comments in the bead description:
- Guidance or clarifications
- Decisions made
- Technical notes or constraints
- Skip routine status updates or acknowledgments

### Linked Confluence Pages
If the ticket links to Confluence pages (design docs, specs, etc.):
1. Fetch the page using `fetch-confluence` skill
2. Extract key information relevant to the task
3. Include summary in bead description

## Parsing Action Items

Check descriptions for actionable items. Create as children of that ticket's bead.

**Only parse items under task-related sections:**
- `TODO`, `Tasks`, `Action Items`, or similar headings
- Do NOT parse items under `QA`, `Testing`, `Notes`, `Questions`, or other non-task sections

**Create subtasks for:**
- `:white_circle:` or `⚪` (open items)
- `- [ ]` unchecked checkboxes

**Skip:**
- `:check_mark:` or `✅` (done)
- `:question_mark:` or `❓` (questions)
- `- [x]` checked boxes (done)

## Example

```bash
# Fetch ticket hierarchy
jira issue view PROJ-456 --raw | jq '{key: .key, summary: .fields.summary, parent: .fields.parent.key}'

# Create beads (see beads skill for syntax)
echo "new 'PROJ-123: Feature epic' 'Epic'" | 9p write agent/beads/ctl
ROOT=$(9p read agent/beads/list | jq -r '.[] | select(.title | contains("PROJ-123")) | .id')
echo "new 'PROJ-456: Feature story' 'Story' $ROOT" | 9p write agent/beads/ctl
```

## Human Review

After import, reviewer may:
- Remove beads bot doesn't need
- Adjust descriptions
- Add context
