---
name: anvillm-communication
intent: messaging, agents
description: Communicate within anvillm via the mailbox system. Use for agent-to-agent messaging (review requests, questions, approvals), agent-to-user responses, discovering peer agents, and any inter-agent coordination.
when_to_load: Load when you need to send messages to other agents, respond to the user, discover peer agents, check your inbox, or coordinate with other agents in a workflow.
---

# Anvillm Communication

## Discovering Agents

List all running agents:
```
execute_code with code: "bash <(9p read agent/tools/anvilmcp/list_sessions.sh)"
```

Returns JSON array with id, name, state, assignee, workdir. Use the `id` field for communication.

## Message Body Schema

Message bodies must be terse structured text, not prose. Receiving agents parse structured output.

**PROMPT_RESPONSE body** — depends on what the PROMPT_REQUEST asked:

- *Question*: just the answer, one line if possible.
- *Task*: use the completion schema:
```
Status: completed | failed | blocked
Beads: bd-abc, bd-xyz  (or none)
Errors: none  (or list errors)
Notes: [only if actionable for the recipient]
```

**QUERY_REQUEST / QUERY_RESPONSE body**: one-line question or answer.

**REVIEW_REQUEST body**: `Bead: bd-abc\nDiff: <file or commit>\nQuestion: <specific question>`

**APPROVAL_REQUEST body**: `Action: <what needs approval>\nRisk: <impact if approved>`

No prose. No preamble. No summaries of completed work.

## Sending Messages

Send a message:
```
execute_code with code: "bash <(9p read agent/tools/anvilmcp/send_message.sh) FROM TO TYPE SUBJECT BODY"
```

Parameters:
- FROM: your AGENT_ID
- TO: recipient session_id or "user"
- TYPE: message type (see below)
- SUBJECT: brief description (≤10 words)
- BODY: structured message content (see schema above)

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
execute_code with code: "bash <(9p read agent/tools/anvilmcp/send_message.sh) YOUR_AGENT_ID user PROMPT_RESPONSE 'Task complete' 'Implemented the feature and ran tests successfully'"
```

**Ask user a question:**
```
execute_code with code: "bash <(9p read agent/tools/anvilmcp/send_message.sh) YOUR_AGENT_ID user QUERY_REQUEST 'Need deployment target' 'Which environment should I deploy to: staging or production?'"
```

**Send work to another agent:**
```
execute_code with code: "bash <(9p read agent/tools/anvilmcp/send_message.sh) YOUR_AGENT_ID abc123def PROMPT_REQUEST 'Review needed' 'Please review the changes in commit xyz789'"
```

## Checking Your Inbox

**Messages are delivered automatically** - you don't need to actively check. The system will deliver incoming messages to you.

To manually check:
```
execute_code with code: "bash <(9p read agent/tools/anvilmcp/read_inbox.sh) YOUR_AGENT_ID"
```

### Responding to Messages

When you receive a message:
1. **Read and understand** the message content
2. **Take the requested action** (answer question, review code, etc.)
3. **Send a response** back to the sender (see examples above)

For `PROMPT_REQUEST` messages, respond with `PROMPT_RESPONSE`.
For `QUERY_REQUEST` messages, respond with `QUERY_RESPONSE`.
For `REVIEW_REQUEST` messages, respond with `REVIEW_RESPONSE`.
For `APPROVAL_REQUEST` messages, respond with `APPROVAL_RESPONSE`.

Example response flow:
```
# Received: PROMPT_REQUEST from agent abc123
# After completing the work, respond:
execute_code with code: "bash <(9p read agent/tools/anvilmcp/send_message.sh) YOUR_AGENT_ID abc123 PROMPT_RESPONSE 'Task completed' 'Status: completed\nBeads: none\nErrors: none'"
```

## Agent State

Set your state:
```
execute_code with code: "bash <(9p read agent/tools/anvilmcp/set_state.sh) YOUR_AGENT_ID STATE"
```

States: idle, running, stopped, starting, error, exited

Check other agents' states via list_sessions output.

## Message Flow

1. **bot → user**: Send message with `to="user"`. Message appears in bot's chat log. Sender transitions to idle.
2. **user → bot**: User sends to bot's session_id. Delivered to bot's inbox automatically.
3. **bot → bot**: Send message with target session_id. Delivered to recipient's inbox automatically.

## Quick Reference

- **List agents**: `bash <(9p read agent/tools/anvilmcp/list_sessions.sh)`
- **Send message**: `bash <(9p read agent/tools/anvilmcp/send_message.sh) FROM TO TYPE SUBJECT BODY`
- **Check inbox**: `bash <(9p read agent/tools/anvilmcp/read_inbox.sh) AGENT_ID`
- **Set state**: `bash <(9p read agent/tools/anvilmcp/set_state.sh) AGENT_ID STATE`
