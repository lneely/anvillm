---
name: anvillm-communication
description: Communicate within anvillm via the mailbox system. Use for agent-to-agent messaging (review requests, questions, approvals), agent-to-user responses, discovering peer agents, and any inter-agent coordination.
when_to_load: Load this skill when you need to send messages to other agents, respond to the user, discover peer agents, check your inbox, or coordinate with other agents in a workflow.
---

# Anvillm Communication

This skill covers all communication in anvillm: discovering agents, sending messages between agents, and communicating with the user.

## Currently Available Agents

Use the `list_sessions` tool to see all running agents.

## Discovering Agents

### List All Agents

To see all currently running agents:
```
list_sessions
```

The output shows one agent per line with the format: `{session_id} {alias} {state} {pid} {cwd}`

### Extract Session ID

From the agent list output, the first field is the session ID you need for communication.

## Sending Messages

All communication uses the mailbox system via the `send_message` tool.

### Choosing the Right Message Type

**When responding to the user:**
1. **Just answering or providing status?** → Use `PROMPT_RESPONSE`
   - "I've completed the task"
   - "Here's the information you requested"
   - "I've made the changes"

2. **Need information from user?** → Use `QUERY_REQUEST`
   - "What should the timeout value be?"
   - "Which environment should I deploy to?"

3. **Need user approval?** → Use `APPROVAL_REQUEST`
   - "Ready to deploy to production. Approve?"
   - "Should I proceed with deleting these files?"

4. **Need user to review something?** → Use `REVIEW_REQUEST`
   - "Please review the changes in commit abc123"
   - "Can you verify this implementation?"

**When working with other agents:**
- Use `PROMPT_REQUEST` to send work requests
- Use `*_REQUEST` / `*_RESPONSE` pairs for structured workflows

### To Another Agent

```
send_message with:
  from: your AGENT_ID
  to: target session_id
  type: PROMPT_REQUEST
  subject: Brief subject
  body: Your message here
```

### To User

The special recipient `"user"` sends messages to the human operator.

**For simple responses (auto-completed, doesn't clutter inbox):**
```
send_message with:
  from: your AGENT_ID
  to: user
  type: PROMPT_RESPONSE
  subject: Task complete
  body: Summary of what was done
```

**When you need user input (stays in inbox until answered):**
```
# Ask a question
send_message with:
  from: your AGENT_ID
  to: user
  type: QUERY_REQUEST
  subject: Need information
  body: What is the target deployment environment?

# Request approval
send_message with:
  from: your AGENT_ID
  to: user
  type: APPROVAL_REQUEST
  subject: Approve deployment
  body: Ready to deploy to production. Approve?

# Request review
send_message with:
  from: your AGENT_ID
  to: user
  type: REVIEW_REQUEST
  subject: Review changes
  body: Please review the implementation in commit abc123
```

### Message Types

**When responding to user prompts:**
- `PROMPT_RESPONSE` - Your answer/status update to user (auto-completed, doesn't clutter inbox)
- `QUERY_REQUEST` - You need information from user (stays in inbox until answered)
- `APPROVAL_REQUEST` - You need user approval for an action (stays in inbox until answered)
- `REVIEW_REQUEST` - You need user to review something (stays in inbox until answered)

**When working with other agents:**
- `PROMPT_REQUEST` - Send work request to another agent
- `QUERY_REQUEST` / `QUERY_RESPONSE` - Ask/answer questions
- `REVIEW_REQUEST` / `REVIEW_RESPONSE` - Request/provide code reviews
- `APPROVAL_REQUEST` / `APPROVAL_RESPONSE` - Request/provide testing approval

**Error reporting:**
- `LOG_ERROR` - Report errors (auto-completed)

**Deprecated (use new types):**
- `LOG_INFO` → use `PROMPT_RESPONSE`
- `PROMPT` → use `PROMPT_REQUEST`
- `STATUS_UPDATE` → use `PROMPT_RESPONSE`
- `ERROR_REPORT` → use `LOG_ERROR`
- `QUESTION` → use `QUERY_REQUEST`
- `ANSWER` → use `QUERY_RESPONSE`

### Example Workflow

1. **Discover the peer**: Use `list_sessions` and look for the agent you need
2. **Extract ID**: Note the session ID (first field)
3. **Send message**: Use `send_message` tool
4. **Check inbox**: Use `read_inbox` tool to see responses

## Mailbox System

### Structure

Each agent has:
- **mail** - Send messages via `send_message` tool
- **inbox** - Check messages via `read_inbox` tool

The special `user` participant also has mailboxes.

### Message Format

Messages include:
- `from` - sender (your AGENT_ID)
- `to` - recipient (session ID or `"user"`)
- `type` - message type
- `subject` - brief description
- `body` - message content

### Message Flow

1. Use `send_message` tool to send
2. System delivers to recipient's inbox
3. For agents: messages are automatically delivered when they arrive
4. For user: message body is written to sender's chat log

### User Communication

- **bot → user**: Use `send_message` with `to="user"`. Message body appears in bot's chat log. Sender transitions to idle.
- **user → bot**: User sends message to bot's session ID. Delivered to bot's inbox.

### Checking for Messages

**You do not need to actively check your inbox.** When messages arrive, the system automatically delivers them to you. Simply continue your work - incoming messages will be delivered automatically.

If you need to manually check:
```
read_inbox with agent_id set to your AGENT_ID
```

### Checking Agent State

```
Use the set_state tool to change your state, or check other agents' states via list_sessions
```

Common states:
- `idle` - Ready to receive work
- `running` - Currently processing
- Other states depend on the agent implementation

## Additional Operations

### Read Agent Alias
Check the `list_sessions` output - the second field is the alias.

### Set Agent Alias
Use the 9p filesystem directly:
```bash
echo "new-alias-name" | 9p write agent/{session_id}/alias
```

### Read Agent Context
```bash
9p read agent/{session_id}/context
```

## Real-World Examples

See the `scripts/` directory in the anvillm project for complete workflow examples:
- **scripts/DevReview**: Developer-reviewer collaboration workflow
- **scripts/Planning**: Research, engineering, and tech-editor workflow

## Communication Patterns

### Request-Response Pattern

```bash
# A discovers B
B_ID=$(9p read agent/list | grep agent-b | awk '{print $1}')
A_ID=$(9p read agent/list | grep agent-a | awk '{print $1}')

# A sends request to B
cat > /tmp/msg.json <<'EOF'
{"to":"$B_ID","type":"PROMPT_REQUEST","subject":"Work request","body":"Please do X"}
EOF
9p write agent/$A_ID/mail < /tmp/msg.json

# Check inbox for B's response
9p ls agent/$A_ID/inbox/
```

### Iterative Review Pattern

```bash
# Developer sends review request
cat > /tmp/msg.json <<'EOF'
{"to":"$REVIEWER_ID","type":"REVIEW_REQUEST","subject":"Code review","body":"Please review staged changes"}
EOF
9p write agent/$DEV_ID/mail < /tmp/msg.json

# Developer checks inbox for response
9p ls agent/$DEV_ID/inbox/
```

### Signaling to User (MANDATORY)

**You MUST send PROMPT_RESPONSE to user at the end of EVERY interaction**, not just when the overall task is complete. This keeps the user informed of progress at each step.

**Use PROMPT_RESPONSE for status updates and responses:**
```bash
cat > /tmp/msg.json <<'EOF'
{"to":"user","type":"PROMPT_RESPONSE","subject":"Response","body":"Summary of what was done in THIS interaction"}
EOF
9p write agent/{your_id}/mail < /tmp/msg.json
```

**Use request types when you need user input:**
- `QUERY_REQUEST` - When you need information from the user
- `APPROVAL_REQUEST` - When you need user approval for an action
- `REVIEW_REQUEST` - When you need user to review something

Examples of when to send PROMPT_RESPONSE to user:
- After implementing changes and sending a review request (before waiting for reviewer)
- After receiving reviewer feedback and making requested changes
- After completing the final step of a task
- After answering a question or providing information

The body should summarize the key actions taken in that specific interaction.

## Important Notes

- The agent list is dynamic - agents can start and stop at any time
- Each agent has a unique session ID
- `"user"` is a special recipient for human communication
- Writing to user automatically transitions sender to idle
- Aliases help identify agents but IDs are required for communication

## Workflow Best Practices

1. **Discovery First**: Always list agents before attempting communication
2. **Check State**: Verify agent is idle before sending work requests (when appropriate)
3. **Clear Instructions**: Include reply address in your message
4. **Error Handling**: Check that grep finds exactly one agent when expecting a specific peer
5. **Aliases**: Use meaningful aliases to identify agents by role
6. **Prompt Responses**: Send PROMPT_RESPONSE to user at the end of EVERY interaction (not just task completion)

## Troubleshooting

**Agent not found**:
- Run `9p read agent/list` to see all agents
- Verify your grep pattern matches the intended agent
- Check that the agent hasn't been closed

**No response from peer**:
- Check peer's state: `9p read agent/{id}/state`
- Verify you sent to the correct session ID
- Check that your message included clear instructions

**Multiple agents match**:
- Use more specific grep patterns
- Combine multiple grep filters
- Use unique aliases to differentiate agents
