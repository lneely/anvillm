---
name: anvillm-communication
intent: messaging, agents
description: Agent discovery and messaging via the mailbox system.
when_to_load: Load when you need to send messages to other agents, respond to the user, discover peer agents, check your inbox, or coordinate with other agents in a workflow.
---

# Anvillm Communication

## Commands

Use `execute_code` tool with these commands:

- List agents: `9p read agent/tools/agents/list_sessions.sh | bash`
- Send message: `9p read agent/tools/messaging/send_message.sh | bash -s FROM TO TYPE SUBJECT BODY`
- Check inbox: `9p read agent/tools/messaging/read_inbox.sh | bash -s AGENT_ID`

## Use Cases

| Goal | TO | TYPE | BODY format |
|------|----|------|-------------|
| Reply to user | user | PROMPT_RESPONSE | `Status: completed\|failed\|blocked\nBeads: bd-xxx\nErrors: none` |
| Ask user question | user | QUERY_REQUEST | one-line question |
| Request user approval | user | APPROVAL_REQUEST | `Action: ...\nRisk: ...` |
| Request user review | user | REVIEW_REQUEST | `Bead: bd-xxx\nDiff: ...\nQuestion: ...` |
| Send work to agent | agent_id | PROMPT_REQUEST | task description |
| Answer agent question | agent_id | QUERY_RESPONSE | one-line answer |

Response types mirror request types: PROMPT→PROMPT, QUERY→QUERY, REVIEW→REVIEW, APPROVAL→APPROVAL.
