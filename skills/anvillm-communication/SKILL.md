---
name: anvillm-communication
intent: messaging, agents, inbox
description: Agent discovery and messaging via the mailbox system.
when_to_load: Load when you need to send messages to other agents, respond to the user, discover peer agents, check your inbox, or coordinate with other agents in a workflow.
---

# Anvillm Communication Skill

## Tools

**IMPORTANT:** Read the $AGENT_ID environment variable to get your agent ID

Discover agents:
```
Tool: execute_code
pipe: [
	{"tool": "list_sessions.sh"},
	{"code": "grep '<current-working-dir>'"}
]
```

Send message:
```
Tool: execute_code
tool: send_message.sh
args: ["<from>", "<to>", "<type>", "<subject>", "<body>"]
```

Check inbox:
```
Tool: execute_code
tool: read_inbox.sh
args: ["<agent_id>"]
```

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
3. Always respond to the sender (the `from` field of the message), not to "user", unless the sender is "user"
