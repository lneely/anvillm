---
name: peer-bot
description: Discover and interact with peer bots in the anvillm 9P filesystem. Use when the user wants to communicate with, send prompts to, or interact with another bot or agent.
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

To send a message to a peer bot:
```bash
echo "Your message here" | 9p write agent/{session_id}/in
```

Where `{session_id}` is the ID obtained from `agent/list`.

### Example Workflow

1. **Discover the peer**: `9p read agent/list | grep research`
2. **Extract ID**: Note the session ID (first field)
3. **Send message**: `echo "Please research X" | 9p write agent/{id}/in`

## Understanding Peer Bot Communication

### Context-Driven Behavior

Each agent has a context (set via `agent/{id}/context`) that defines:
- The agent's role and responsibilities
- How it should handle incoming messages
- Which peers it should communicate with
- The communication protocol to follow

### Message Flow

When you send a message via `agent/{id}/in`:
1. The message is added to the agent's input stream
2. Any injected context is automatically prepended
3. The agent processes the message according to its role
4. The agent may respond by writing to another agent's input

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

# A sends request to B
echo "Please do X and reply to agent/$A_ID/in" | 9p write agent/$B_ID/in

# B will eventually respond by writing to agent/$A_ID/in
```

### Iterative Review Pattern

Developer submits work, reviewer provides feedback:
```bash
# Developer sends review request
echo "Please review staged changes" | 9p write agent/$REVIEWER_ID/in

# Reviewer responds with LGTM or feedback
echo "LGTM" | 9p write agent/$DEV_ID/in
# or
echo "Please fix: ..." | 9p write agent/$DEV_ID/in
```

## Important Notes

- The agent list is dynamic - agents can start and stop at any time
- Each agent has a unique session ID
- Aliases help identify agents but IDs are required for communication
- All peer interactions use the 9p filesystem interface
- Context injection allows complex agent behaviors without hard-coding logic
- Agents should include reply instructions in their messages (e.g., "respond to agent/{id}/in")
- Always verify an agent exists before attempting to send messages
- Use `grep` to filter agents by role, alias, or other identifiers

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
