# IDEAS.md

## Issue Tracker Import

Import issues from Jira, GitHub, GitLab, or Linear into beads for agent-driven work.

### Supported Trackers

Beads library already has importers for:
- **Jira** (`internal/jira/`) - Epics, stories, subtasks
- **GitHub** (`internal/github/`) - Issues, PRs, milestones  
- **GitLab** (`internal/gitlab/`) - Issues, merge requests, epics
- **Linear** (`internal/linear/`) - Issues, projects, cycles

### Usage

```sh
# Jira
echo 'import-jira PROJ-123 --recursive' | 9p write agent/beads/ctl

# GitHub
echo 'import-github owner/repo#123' | 9p write agent/beads/ctl
echo 'import-github owner/repo milestone:v1.0' | 9p write agent/beads/ctl

# GitLab
echo 'import-gitlab group/project#456' | 9p write agent/beads/ctl

# Linear
echo 'import-linear TEAM-789' | 9p write agent/beads/ctl
```

### Mapping

| Tracker | Beads |
|---------|-------|
| Jira Epic / GitHub Milestone / Linear Project | Issue with type=epic |
| Story / Issue / Task | Issue with type=feature/task/bug |
| Subtask | Issue with parent dependency |
| Blocks / Depends on | Dependency with type=blocks |
| PR / MR | Issue with type=task, linked to parent issue |

### Pull-Only Sync

All trackers use pull-only sync - tracker is source of truth:

```sh
# Refresh from tracker
echo 'refresh-github bd-a1b2' | 9p write agent/beads/ctl
echo 'refresh-jira bd-xyz' | 9p write agent/beads/ctl
```

Auto-refresh:
```yaml
# .beads/config.yaml
sync:
  auto_refresh: true
  interval: 300s
  
jira:
  url: https://company.atlassian.net
  token: ${JIRA_TOKEN}
  
github:
  token: ${GITHUB_TOKEN}
  
gitlab:
  url: https://gitlab.company.com
  token: ${GITLAB_TOKEN}
  
linear:
  token: ${LINEAR_TOKEN}
```

### Benefits

- **Unified interface**: Agents use beads regardless of tracker
- **Cross-tracker**: Import from multiple trackers into one bead database
- **Safe**: Pull-only, no risk of corrupting tracker data
- **Dependency tracking**: Tracker links become bead dependencies

### Example: Multi-Tracker Workflow

```sh
# Import Jira epic for backend work
echo 'import-jira BACKEND-100 --recursive' | 9p write agent/beads/ctl

# Import GitHub issues for frontend work  
echo 'import-github company/frontend milestone:v2.0' | 9p write agent/beads/ctl

# Agent sees unified view
9p read agent/beads/ready
# bd-a1b2: BACKEND-101 - API endpoint
# bd-c3d4: company/frontend#45 - UI component

# Agent works on both, regardless of source tracker
```

## Pull-Only Sync Implementation

## Approval Gates

Sessions pause for human approval before proceeding.

### Use Case

Bot completes a task but shouldn't continue until work is reviewed:
1. Developer bot implements feature
2. Bot sends approval request via mailbox to human notification system
3. Human receives notification (desktop, Signal, etc.)
4. Human reviews work and responds with approval/rejection
5. Bot receives response and continues or stops

### Implementation with Mailbox System

Use existing `APPROVAL_REQUEST` and `APPROVAL_RESPONSE` message types:

```bash
# Bot sends approval request
cat > /tmp/approval.json <<EOF
{"to":"human","type":"APPROVAL_REQUEST","subject":"OAuth implementation","body":"Implemented OAuth login. Added 3 files, 200 LOC. All tests pass. Please review and approve."}
EOF
9p write agent/dev-123/mail < /tmp/approval.json

# Bot waits (stays idle, checks inbox for response)
```

### Human Notification System

External daemon monitors special "human" inbox or approval queue:
- Watches for `APPROVAL_REQUEST` messages
- Sends desktop notification or Signal message
- Presents approval UI (approve/reject with reason)
- Writes response back to bot's inbox

```bash
# Human approves (via notification UI or manual command)
cat > /tmp/response.json <<EOF
{"to":"dev-123","type":"APPROVAL_RESPONSE","subject":"Approved","body":"LGTM - OAuth implementation looks good. Proceed with deployment."}
EOF
9p write agent/human/mail < /tmp/response.json
```

### Bot Behavior

Bot context includes:
```
After completing critical tasks:
1. Send APPROVAL_REQUEST message to "human"
2. Wait for APPROVAL_RESPONSE in your inbox
3. If approved, continue with next step
4. If rejected, address feedback and request approval again
```

Bot checks inbox periodically or waits for mail processor to deliver response.

### Integration with Beads

Beads can require approval before marking complete:
```toml
[bead]
requires_approval = true
approved_by = ""  # Session ID or "human"
```

Bot completes bead but status stays `pending_approval` until approved.

### Philosophy: Human in the Loop

Approval gates ensure precision:
- Critical changes reviewed before proceeding
- Prevents cascading errors from bad decisions
- Human maintains control over workflow direction
- Bot can work autonomously within approved boundaries
- Notification system brings approvals to human's attention

### Status

Mailbox system implemented. Needs:
- Human notification daemon (desktop/Signal integration)
- Bot context patterns for approval workflows
- UI for approval responses

## Convoy (Grouped Work)

Collection of related beads working toward a common goal.

### Concept

Convoy = group of beads + shared context + coordinator

```toml
[convoy]
id = "auth-system-001"
title = "Implement authentication system"
goal = "Add OAuth and session management to application"
status = "in_progress"  # planning | in_progress | completed | failed
coordinator = "planner-bot-789"
created_at = 1739587200
updated_at = 1739590800
beads = [
  "research-auth-001",
  "impl-oauth-002",
  "write-tests-003",
  "review-security-004"
]
```

### 9P Integration

```
agent/
├── convoys/
│   ├── ctl              # "new <title> <goal>" creates convoy
│   ├── list             # All convoys with status
│   └── <convoy-id>/
│       ├── title
│       ├── goal
│       ├── status
│       ├── coordinator  # Session ID managing this convoy
│       ├── beads        # Newline-separated bead IDs
│       ├── context      # Shared context for all beads in convoy
│       └── ctl          # "add_bead <id>", "complete", "abandon"
```

### Workflow

1. **Create convoy**: Planner bot or human defines high-level goal
2. **Break down**: Coordinator creates beads for convoy
3. **Execute**: Worker bots claim and complete beads
4. **Coordinate**: Coordinator monitors progress, creates new beads as needed
5. **Complete**: All beads done, convoy marked complete

### Example

```sh
# Create convoy
echo 'new "Implement auth system" "Add OAuth and session management"' | 9p write agent/convoys/ctl

# Coordinator creates beads
echo 'new "Research auth" researcher "Analyze requirements"' | 9p write agent/beads/ctl
echo 'add_bead research-auth-001' | 9p write agent/convoys/auth-system-001/ctl

echo 'new "Implement OAuth" developer "Add OAuth based on research" research-auth-001' | 9p write agent/beads/ctl
echo 'add_bead impl-oauth-002' | 9p write agent/convoys/auth-system-001/ctl

# Workers execute beads
# Coordinator monitors, adds more beads if needed
# Convoy completes when all beads done
```

### Shared Context

All beads in convoy inherit convoy context:
```
When working on beads in this convoy, remember:
- Target: web application with React frontend
- Security: OWASP top 10 compliance required
- Timeline: 2 weeks
- Constraints: Must integrate with existing user database
```

### Benefits

- **Cohesion**: Related work grouped together
- **Context sharing**: All workers understand overall goal
- **Progress tracking**: See convoy completion percentage
- **Coordination**: Coordinator adjusts plan based on results
- **Resumability**: Convoy persists across crashes

### Philosophy: Deliberate Orchestration

Convoys enable precision through:
- **Explicit goals**: Clear definition of what success looks like
- **Adaptive planning**: Coordinator adjusts based on results
- **Shared understanding**: All workers have convoy context
- **Traceable decisions**: Why each bead was created

Unlike Gas Town's "spawn workers and hope", convoys are deliberate, coordinated, and traceable.

## Conductor Bot

A meta-bot that orchestrates workflows by watching status and dispatching prompts.

### Role

- Monitors status files of worker bots
- Issues coordination prompts when bots enter `await`
- Defines workflow through prompt injection, not hardcoded logic

### Example: Adversarial Review Cycle

Conductor's initial prompt:
```
You are coordinating a code review cycle between 5 reviewer bots and 1 dev bot.

Workflow:
1. Tell reviewers: "Review the code in dev/out, write feedback to your out"
2. Wait for all 5 reviewers to enter await
3. Tell dev: "Read reviewer1/out through reviewer5/out, apply feedback, write updated diff to out"
4. Wait for dev to enter await
5. Repeat from step 1 until reviewers report no issues

Monitor status files. Only proceed when all expected bots are in await.
```

Conductor reads status, sends prompts to `in` files, watches for `await`. The workflow is entirely prompt-driven.

### Benefits

- Workflows defined in natural language, not code
- Easy to modify - just change conductor's prompt
- Conductor can be swapped, forked, or layered (conductor of conductors)
- Worker bots stay simple - they just do tasks and write to `out`

### Conductor as Bot vs External Process

Could be:
- Another LLM bot (flexible, can adapt, but slower and uses tokens)
- A simple script that polls status and sends canned prompts (fast, deterministic)
- Hybrid: script handles timing, bot handles decisions when workflow branches

### 9p-Native Conductor

9p makes coordination trivial. Conductor just reads/writes files - no IPC, no sockets, no message queues. The filesystem *is* the coordination API.

Pseudocode:
```
while true:
    for bot in bots:
        status = read(bot/status)
        if status == "await" and bot in pending:
            next = workflow.next(bot)
            write(next.bot/in, next.prompt)
            pending.remove(bot)
            pending.add(next.bot)
    sleep(poll_interval)
```

Could be 50 lines of rc, Python, or Go. Workflow definitions could be:
- Hardcoded in the conductor
- Loaded from a config file
- Defined in a `workflow` file in 9p itself (conductor reads its own instructions from the filesystem)

## Ephemeral Sessions

Task-oriented sessions that perform a single action then stop automatically.

### Lifecycle

`idle` → `running` → `stopped`

No persistent interaction. Bot receives prompt, executes, writes result to `out`, transitions to `stopped`.

### Design Questions

- **Restartable?** Should `stopped` ephemeral sessions support `restart`, or only `kill`? If restartable, they become regular sessions. If not, simpler lifecycle but less flexible.
- **Non-interactive?** Could enforce no tmux attachment, or allow attachment for debugging but expect no human input.
- **Kill vs Stop?** Maybe ephemeral sessions go straight to `exited` instead of `stopped`. Or use `stopped` but auto-cleanup after some timeout.

### Use Cases

- One-off code reviews: "review this diff" → done
- Batch processing: spawn 10 ephemeral sessions, each processes one file
- Fire-and-forget queries: "what's the status of X?" → answer in `out`, session dies

### Implementation

Flag in session creation: `echo 'new claude /project ephemeral' | 9p write agent/ctl`

Daemon marks session as ephemeral. When backend exits cleanly (not crash), transition to `stopped` or `exited` instead of auto-restart. Optionally auto-cleanup after N seconds.
