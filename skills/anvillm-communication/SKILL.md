---
name: anvillm-communication
description: Communicate within anvillm via the mailbox system. Use for agent-to-agent messaging (review requests, questions, approvals), agent-to-user responses, discovering peer agents, and any inter-agent coordination.
when_to_load: Load when you need to send messages to other agents, respond to the user, discover peer agents, check your inbox, or coordinate with other agents in a workflow.
---

# Anvillm Communication

## Discovering Agents

Use `list_sessions` to see all running agents. Output format: `{session_id} {alias} {state} {pid} {cwd}`

The session_id (first field) is what you need for communication.

## Sending Messages

Use the `send_message` tool with these parameters:
- `from`: your AGENT_ID
- `to`: recipient session_id or "user"
- `type`: message type (see below)
- `subject`: brief description
- `body`: message content

### Message Types

**To User:**
- `PROMPT_RESPONSE` - Status update/answer to user prompt
- `QUERY_REQUEST` - Need information from user (stays until answered)
- `APPROVAL_REQUEST` - Need user approval (stays until answered)
- `REVIEW_REQUEST` - Need user to review something (stays until answered)

**To Other Agents:**
- `PROMPT_REQUEST` - Send work request
- `QUERY_REQUEST` / `QUERY_RESPONSE` - Ask/answer questions
- `REVIEW_REQUEST` / `REVIEW_RESPONSE` - Request/provide reviews
- `APPROVAL_REQUEST` / `APPROVAL_RESPONSE` - Request/provide approvals


### Examples

**Simple response to user:**
```
send_message with:
  from: your AGENT_ID
  to: user
  type: PROMPT_RESPONSE
  subject: Task complete
  body: Implemented the feature and ran tests successfully
```

**Ask user a question:**
```
send_message with:
  from: your AGENT_ID
  to: user
  type: QUERY_REQUEST
  subject: Need deployment target
  body: Which environment should I deploy to: staging or production?
```

**Send work to another agent:**
```
send_message with:
  from: your AGENT_ID
  to: abc123def
  type: PROMPT_REQUEST
  subject: Review needed
  body: Please review the changes in commit xyz789
```

## Checking Your Inbox

**Messages are delivered automatically** - you don't need to actively check. The system will deliver incoming messages to you.

To manually check: `read_inbox` with `agent_id` set to your AGENT_ID

### Responding to Messages

When you receive a message:
1. **Read and understand** the message content
2. **Take the requested action** (answer question, review code, etc.)
3. **Send a response** back to the sender using `send_message`

For `PROMPT_REQUEST` messages, respond with `PROMPT_RESPONSE`.
For `QUERY_REQUEST` messages, respond with `QUERY_RESPONSE`.
For `REVIEW_REQUEST` messages, respond with `REVIEW_RESPONSE`.
For `APPROVAL_REQUEST` messages, respond with `APPROVAL_RESPONSE`.

Example response flow:
```
# Received: PROMPT_REQUEST from agent abc123
# After completing the work, respond:
send_message with:
  from: your AGENT_ID
  to: abc123
  type: PROMPT_RESPONSE
  subject: Task completed
  body: I've completed the requested work. Details: ...
```

## Agent State

Use `set_state` to change your state:
- `idle` - Ready for work
- `running` - Currently processing
- `stopped` - Paused
- `starting` - Initializing
- `error` - Error state
- `exited` - Terminated

Check other agents' states via `list_sessions` output (third field).

## Message Flow

1. **bot → user**: Use `send_message` with `to="user"`. Message appears in bot's chat log. Sender transitions to idle.
2. **user → bot**: User sends to bot's session_id. Delivered to bot's inbox automatically.
3. **bot → bot**: Use `send_message` with target session_id. Delivered to recipient's inbox automatically.

## Quick Reference

- **List agents**: `list_sessions`
- **Send message**: `send_message` (from, to, type, subject, body)
- **Check inbox**: `read_inbox` (agent_id)
- **Set state**: `set_state` (agent_id, state)
