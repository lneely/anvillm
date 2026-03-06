---
name: anvillm-communication
intent: messaging, agents, inbox
description: Agent discovery and messaging via the mailbox system.
when_to_load: Load when you need to send messages to other agents, respond to the user, discover peer agents, check your inbox, or coordinate with other agents in a workflow.
---

# Anvillm Communication Skill

## Tools

**IMPORTANT:** All anvillm-communication tools must be run via `execute_code` tool.
**IMPORTANT:** Read the $AGENT_ID environment variable to get your agent ID

- Discover agents: `bash <(9p read agent/tools/agents/list_sessions.sh) | grep "$(pwd)"`
- Send message: `bash <(9p read agent/tools/messaging/send_message.sh) <FROM> <TO> <TYPE> <SUBJECT> <BODY>`
- Check inbox: `bash <(9p read agent/tools/messaging/read_inbox.sh) <AGENT_ID>`

## Rules

### Discovering Agents

1. You communicate only with "user", or other agents working in the same $(pwd) as yourself

### Sending Messages

1. FROM is your agent ID
2. TO is the agent ID of the receiving bot, or "user"
3. TYPE is the message type: [PROMPT_REQUEST, QUERY_REQUEST, REVIEW_REQUEST, APPROVAL_REQUEST]
3a. Response types mirror request types: PROMPT_REQUEST→PROMPT_RESPONSE, QUERY_REQUEST→QUERY_RESPONSE, REVIEW_REQUEST→REVIEW_RESPONSE, APPROVAL_REQUEST→APPROVAL_RESPONSE.
4. SUBJECT is a brief description of what you did
5. BODY is a detailed summary of what you did, including actions performed, files changed, diffs, etc.

### Receiving Messages

1. Check your inbox
2. Follow the instructions in the message
3. Respond to the sender appropriately (see Sending Messages)


