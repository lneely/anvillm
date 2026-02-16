---
name: anvillm-communication
description: Communicate within anvillm via the 9P mailbox system. Use for agent-to-agent messaging (review requests, questions, approvals), agent-to-user responses, discovering peer agents, and any inter-agent coordination.
---

# Anvillm Communication

This skill covers all communication in anvillm: discovering agents, sending messages between agents, and communicating with the user.

## Currently Available Agents

`9p read agent/list`

## Discovering Agents

### List All Agents

To see all currently running agents:
```bash
9p read agent/list
```

The output shows one agent per line with the format: `{session_id} {alias} {other_info}`

### Find Specific Agent by Alias

To find an agent by its alias name:
```bash
9p read agent/list | grep {alias_name}
```

You can combine multiple filters:
```bash
9p read agent/list | grep reviewer | grep alpha01
```

This finds agents with both "reviewer" and "alpha01" in their information.

### Extract Session ID

From the agent list output, the first field is the session ID you need for communication.

## Sending Messages

All communication uses the mailbox system. Write JSON messages to your outbox.

### To Another Agent

```bash
cat > /tmp/msg.json <<'EOF'
{"to":"{session_id}","type":"QUESTION","subject":"Brief subject","body":"Your message here"}
EOF
9p write agent/{your_id}/outbox/msg-$(date +%s).json < /tmp/msg.json
```

### To User

The special recipient `"user"` sends messages to the human operator. Writing to user also transitions your agent to idle state automatically.

```bash
cat > /tmp/msg.json <<'EOF'
{"to":"user","type":"STATUS_UPDATE","subject":"Task complete","body":"Summary of what was done"}
EOF
9p write agent/{your_id}/outbox/msg-$(date +%s).json < /tmp/msg.json
```

### Message Types

- `QUESTION` - Ask for information
- `REVIEW_REQUEST` - Request code review
- `APPROVAL_REQUEST` - Request approval
- `STATUS_UPDATE` - Notify of status change

### Example Workflow

1. **Discover the peer**: `9p read agent/list | grep research`
2. **Extract ID**: Note the session ID (first field)
3. **Create message**: Write JSON to your outbox
4. **Check inbox**: `9p ls agent/{your_id}/inbox/` for responses

## Mailbox System

### Structure

Each agent has:
- **outbox/** - Write messages here to send
- **inbox/** - Receive messages from others
- **completed/** - Archive of processed messages

The special `user` participant also has mailboxes at `agent/user/inbox`, `agent/user/outbox`, `agent/user/completed`.

### Message Format

Messages are JSON files with:
- `to` - recipient (session ID or `"user"`)
- `type` - message type
- `subject` - brief description
- `body` - message content

### Message Flow

1. Write message JSON to your outbox
2. Mail processor (runs every 5s) picks it up
3. Delivers to recipient's inbox
4. For agents: when idle, message is formatted and sent to their `in` file
5. For user: message body is written to sender's chat log
6. Processed messages move to completed/

### User Communication

- **bot → user**: Bot writes to its outbox with `to="user"`. Message body appears in bot's chat log. Sender transitions to idle.
- **user → bot**: User writes to `agent/user/outbox` with `to="{session_id}"`. Delivered to bot's inbox.

### Checking for Messages

```bash
9p ls agent/{your_id}/inbox/
9p read agent/{your_id}/inbox/msg-{id}.json
```

### Checking Agent State

```bash
9p read agent/{session_id}/state
```

Common states:
- `idle` - Ready to receive work
- Other states depend on the agent implementation

## Additional Operations

### Read Agent Alias
```bash
9p read agent/{session_id}/alias
```

### Set Agent Alias
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
{"to":"$B_ID","type":"QUESTION","subject":"Work request","body":"Please do X"}
EOF
9p write agent/$A_ID/outbox/msg-$(date +%s).json < /tmp/msg.json

# Check inbox for B's response
9p ls agent/$A_ID/inbox/
```

### Iterative Review Pattern

```bash
# Developer sends review request
cat > /tmp/msg.json <<'EOF'
{"to":"$REVIEWER_ID","type":"REVIEW_REQUEST","subject":"Code review","body":"Please review staged changes"}
EOF
9p write agent/$DEV_ID/outbox/msg-$(date +%s).json < /tmp/msg.json

# Developer checks inbox for response
9p ls agent/$DEV_ID/inbox/
```

### Signaling to User (MANDATORY)

**You MUST send a STATUS_UPDATE to user at the end of EVERY interaction**, not just when the overall task is complete. This keeps the user informed of progress at each step.

```bash
cat > /tmp/msg.json <<'EOF'
{"to":"user","type":"STATUS_UPDATE","subject":"Response","body":"Summary of what was done in THIS interaction"}
EOF
9p write agent/{your_id}/outbox/msg-$(date +%s).json < /tmp/msg.json
```

Examples of when to send STATUS_UPDATE to user:
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
- All interactions use the mailbox system (outbox/inbox/completed)
- Mail processor delivers messages every 5 seconds
- Messages to agents are delivered when recipient is idle
- Failed deliveries retry up to 3 times, then are discarded

## Workflow Best Practices

1. **Discovery First**: Always list agents before attempting communication
2. **Check State**: Verify agent is idle before sending work requests (when appropriate)
3. **Clear Instructions**: Include reply address in your message
4. **Error Handling**: Check that grep finds exactly one agent when expecting a specific peer
5. **Aliases**: Use meaningful aliases to identify agents by role
6. **Status Updates**: Send STATUS_UPDATE to user at the end of EVERY interaction (not just task completion)

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
