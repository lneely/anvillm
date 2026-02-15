---
name: peer-bot
description: Discover, list, find, send messages to, communicate with, or interact with other agents and bots in anvillm. Use for inter-agent communication including review requests, questions, approvals, status updates, or any message to peer agents like reviewer, developer, architect, QA, researcher, editor, or tester roles.
---

# Peer Bot Interaction

This skill enables you to discover and interact with peer bots running in the anvillm 9P filesystem. Peer bots are defined as any bot working in the same working directory as your own (Cwd column in the agents list).

## Currently Available Agents

`9p read agent/list`

## Discovering Peer Bots

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

## Sending Messages to Peer Bots

To send a message to a peer bot, create a JSON message in your outbox:

```bash
cat > /tmp/msg.json <<EOF
{
  "to": "{session_id}",
  "type": "QUESTION",
  "subject": "Brief subject",
  "body": "Your message here"
}
EOF
9p write agent/{your_id}/outbox/msg-$(date +%s).json < /tmp/msg.json
```

Message types:
- `QUESTION` - Ask for information
- `REVIEW_REQUEST` - Request code review
- `APPROVAL_REQUEST` - Request approval
- `STATUS_UPDATE` - Notify of status change

The mail system will automatically deliver your message to the recipient's inbox.

### Example Workflow

1. **Discover the peer**: `9p read agent/list | grep research`
2. **Extract ID**: Note the session ID (first field)
3. **Create message**: Write JSON to your outbox
4. **Check inbox**: `9p ls agent/{your_id}/inbox/` for responses

## Understanding Peer Bot Communication

### Mailbox System

Agents communicate via structured messages in mailboxes:
- **outbox/** - Write messages here to send to other agents
- **inbox/** - Receive messages from other agents
- **completed/** - Archive of processed messages

Messages are JSON files with:
- `to` - recipient session ID
- `type` - message type (QUESTION, REVIEW_REQUEST, etc.)
- `subject` - brief description
- `body` - message content

### Message Flow

When you write a message to your outbox:
1. Mail processor (runs every 5s) picks it up
2. Delivers to recipient's inbox
3. When recipient is idle, formats and sends to their `in` file
4. Recipient processes and can reply via their outbox
5. Processed messages move to completed/

### Checking for Messages

Check your inbox for responses:
```bash
9p ls agent/{your_id}/inbox/
9p read agent/{your_id}/inbox/msg-{id}.json
```

### Checking Agent State

Before sending a message, you can check if an agent is idle:
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

These scripts demonstrate:
- Creating multi-agent workflows
- Setting up agent contexts with peer communication instructions
- Discovering peers using grep patterns
- Implementing request-response communication patterns

## Communication Patterns

### Request-Response Pattern

Agent A requests work from Agent B:
```bash
# A discovers B
B_ID=$(9p read agent/list | grep agent-b | awk '{print $1}')
A_ID=$(9p read agent/list | grep agent-a | awk '{print $1}')

# A sends request to B via mailbox
cat > /tmp/msg.json <<EOF
{
  "to": "$B_ID",
  "type": "QUESTION",
  "subject": "Work request",
  "body": "Please do X"
}
EOF
9p write agent/$A_ID/outbox/msg-$(date +%s).json < /tmp/msg.json

# Check inbox for B's response
9p ls agent/$A_ID/inbox/
```

### Iterative Review Pattern

Developer submits work, reviewer provides feedback:
```bash
# Developer sends review request
cat > /tmp/review.json <<EOF
{
  "to": "$REVIEWER_ID",
  "type": "REVIEW_REQUEST",
  "subject": "Code review needed",
  "body": "Please review staged changes"
}
EOF
9p write agent/$DEV_ID/outbox/msg-$(date +%s).json < /tmp/review.json

# Reviewer responds via their outbox
# Developer checks inbox for response
9p ls agent/$DEV_ID/inbox/
```

## Important Notes

- The agent list is dynamic - agents can start and stop at any time
- Each agent has a unique session ID
- Aliases help identify agents but IDs are required for communication
- All peer interactions use the mailbox system (outbox/inbox/completed)
- Messages are JSON files with structured fields
- Mail processor delivers messages every 5 seconds
- Messages are delivered when recipient is idle
- Failed deliveries retry up to 3 times, then are discarded
- Always check your inbox for responses
- Use appropriate message types for clarity

## Workflow Best Practices

1. **Discovery First**: Always list agents before attempting communication
2. **Check State**: Verify agent is idle before sending work requests (when appropriate)
3. **Clear Instructions**: Include reply address in your message
4. **Error Handling**: Check that grep finds exactly one agent when expecting a specific peer
5. **Aliases**: Use meaningful aliases to identify agents by role (e.g., "alpha01-dev", "alpha01-reviewer")

## Troubleshooting

**Agent not found**:
- Run `9p read agent/list` to see all agents
- Verify your grep pattern matches the intended agent
- Check that the agent hasn't been closed

**No response from peer**:
- Check peer's state: `9p read agent/{id}/state`
- Verify you sent to the correct session ID
- Check that your message included clear instructions
- Review the peer's context to ensure it's configured to respond

**Multiple agents match**:
- Use more specific grep patterns
- Combine multiple grep filters
- Use unique aliases to differentiate agents
