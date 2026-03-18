# Code Execution User Guide

## Overview

The `execute_code` tool enables agents to write and execute bash scripts in a isolated subprocess. This reduces token consumption by 80-99% compared to direct tool calls by enabling progressive tool discovery and in-subprocess data processing.

## Why Use Code Execution?

### Problem 1: Tool Definitions Consume Context

Loading 100 tools upfront costs ~9,000 tokens (45% of a 200k context window). With code execution, you discover tools progressively:

```bash
# List available tools (~30 tokens)
9p ls anvillm/tools/anvilmcp

# Read only the tool you need (~90 tokens)
9p read anvillm/tools/anvilmcp/check_inbox.sh
```

**Savings**: 97% (120 tokens vs 9,000 tokens)

### Problem 2: Large Data Flows Through Context

Processing a 10,000-row spreadsheet directly costs ~200,000 tokens. With code execution, filter in the subprocess:

```bash
all_rows=$(get_sheet "abc123")
pending=$(echo "$all_rows" | jq '[.[] | select(.Status == "pending")]')
count=$(echo "$pending" | jq 'length')
echo "Found $count pending orders"
# Agent sees: "Found 47 pending orders" (~10 tokens)
```

**Savings**: 99.995% (10 tokens vs 200,000 tokens)

## Basic Usage

### Tool Discovery Workflow

1. **List available tools**:
```bash
9p ls anvillm/tools/anvilmcp
```

2. **Read tool documentation**:
```bash
9p read anvillm/tools/anvilmcp/check_inbox.sh
```

3. **Call tool via MCP**:
```bash
echo '{"name":"read_inbox","arguments":{"agent_id":"82b93a8a"}}' | 9p write anvillm/mcp/call
```

### Calling 9P Operations

You can also call 9P operations directly:

```bash
beads=$(9p read anvillm/beads/list)
count=$(echo "$beads" | jq 'length')
echo "Found $count beads"
```

## Common Patterns

### Pattern 1: Filtering Large Datasets

```bash
# Call read_inbox via MCP
result=$(echo '{"name":"read_inbox","arguments":{"agent_id":"82b93a8a"}}' | 9p write anvillm/mcp/call)
total=$(echo "$result" | jq -r '.content[0].text' | wc -l)
urgent=$(echo "$result" | jq -r '.content[0].text' | grep -c "URGENT")

echo "Found $urgent urgent messages out of $total total"
```

### Pattern 2: Polling Loops

```bash
found=false
attempts=0

while [ $attempts -lt 10 ] && [ "$found" = false ]; do
  sessions=$(anvillm/tools/anvilmcp/list_sessions.sh)
  if echo "$sessions" | jq -e '.[] | select(.state == "completed")' > /dev/null; then
    found=true
  else
    echo "Attempt $((attempts + 1)): Not ready yet"
    sleep 5
  fi
  attempts=$((attempts + 1))
done

if [ "$found" = true ]; then
  echo "Session completed"
else
  echo "Timeout after 10 attempts"
fi
```

### Pattern 3: Multi-Step Aggregations

```bash
agents=("agent-1" "agent-2" "agent-3")
total_messages=0

for agent_id in "${agents[@]}"; do
  result=$(anvillm/tools/anvilmcp/check_inbox.sh "$agent_id")
  count=$(echo "$result" | wc -l)
  total_messages=$((total_messages + count))
done

echo "Total messages across ${#agents[@]} agents: $total_messages"
```

### Pattern 4: Privacy-Preserving Operations

Process sensitive data without exposing it to the model:

```bash
# Read PII from one system
emails=$(9p read anvillm/data/customer-emails)

# Write to another system without PII entering context
echo "$emails" | jq -c '.[]' | while read -r email; do
  address=$(echo "$email" | jq -r '.address')
  name=$(echo "$email" | jq -r '.name')
  
  echo "{\"email\":\"$address\",\"name\":\"$name\"}" | \
    9p write anvillm/crm/contacts
done

count=$(echo "$emails" | jq 'length')
echo "Processed $count contacts"
```

## Available Tools

All tools are accessed via 9P commands. Read tool documentation first:

```bash
9p read anvillm/tools/anvilmcp/check_inbox.sh
```

### Communication

**read_inbox**: Read messages from agent's inbox
```bash
# List inbox messages
9p ls anvillm/AGENT_ID/inbox

# Read specific message
9p read anvillm/AGENT_ID/inbox/MESSAGE_ID.json
```

**send_message**: Send message to another agent or user
```bash
echo '{"from":"agent-1","to":"agent-2","type":"PROMPT_REQUEST","subject":"Task","body":"Do work"}' | \
  9p write anvillm/agent-1/mail
```

### Session Management

**list_sessions**: List all active sessions
```bash
9p read anvillm/list
```

**set_state**: Set agent state
```bash
echo "running" | 9p write anvillm/AGENT_ID/state
```

### Skills

**list_skills**: List available skills
```bash
anvillm-skills list
```

## Error Handling

Always handle errors in your scripts:

```bash
if output=$(9p read anvillm/beads/list 2>&1); then
  echo "$output"
else
  echo "9P command failed: $output" >&2
  exit 1
fi
```

## Subprocess Restrictions

Your code runs in a isolated subprocess with limited permissions:

### Allowed
- Read from `./anvillm/tools/` directory
- Execute `/usr/bin/9p` command
- Access environment variables: `NAMESPACE`, `AGENT_ID`
- Standard bash utilities (jq, grep, sed, awk)

### Blocked
- Network access
- Filesystem writes outside workspace
- Executing arbitrary commands beyond allowed list
- Dangerous patterns: `rm -rf`, `:(){ :|:& };:`, eval with untrusted input

## Performance Tips

### 1. Minimize Tool Discovery

Cache tool paths instead of re-reading:

```bash
# Bad: Re-read every time
for i in {1..10}; do
  9p read anvillm/tools/anvilmcp/check_inbox.sh
done

# Good: Call directly
for i in {1..10}; do
  anvillm/tools/anvilmcp/check_inbox.sh "82b93a8a"
done
```

### 2. Process Data in Batches

```bash
agents=("agent-1" "agent-2" "agent-3")
for agent_id in "${agents[@]}"; do
  anvillm/tools/anvilmcp/check_inbox.sh "$agent_id" &
done
wait
echo "Processed ${#agents[@]} inboxes"
```

### 3. Return Summaries, Not Raw Data

```bash
# Bad: Return all data
result=$(anvillm/tools/anvilmcp/check_inbox.sh "82b93a8a")
echo "$result"  # Could be 50,000 tokens

# Good: Return summary
result=$(anvillm/tools/anvilmcp/check_inbox.sh "82b93a8a")
count=$(echo "$result" | wc -l)
echo "Inbox has $count messages"  # ~10 tokens
```

## Debugging

### View Execution Output

The `execute_code` tool returns stdout and stderr:

```bash
echo "Debug: Starting execution"
echo "Warning: This is a warning" >&2
```

### Check 9P Command Results

```bash
if output=$(9p read anvillm/beads/list 2>&1); then
  count=$(echo "$output" | jq 'length')
  echo "Found $count beads"
else
  echo "9P command failed: $output" >&2
  exit 1
fi
```

## Token Savings Examples

| Scenario | Direct Tools | Code Execution | Savings |
|----------|--------------|----------------|---------|
| List 5 tools | 452 tokens | 30 tokens | 93% |
| List 100 tools | 9,000 tokens | 30 tokens | 99.7% |
| Filter 10k rows | 200,000 tokens | 100 tokens | 99.95% |
| Poll 10 times | 5,000 tokens | 200 tokens | 96% |
| Process 3 inboxes | 15,000 tokens | 150 tokens | 99% |

## Best Practices

1. **Discover tools progressively**: Only read tools you need
2. **Filter data in subprocess**: Return summaries, not raw data
3. **Use loops for repetitive tasks**: Avoid alternating between tool calls and reasoning
4. **Handle errors gracefully**: Always check exit codes
5. **Log meaningful output**: Help the agent understand what happened
6. **Respect subprocess limits**: Don't try to bypass restrictions
7. **Keep scripts simple**: Minimal, focused implementations

## Migration from Direct Tools

### Before (Direct Tool Calls)

```
Agent: TOOL CALL read_inbox(agent_id: "82b93a8a")
MCP: { "content": [{"type": "text", "text": "[50,000 tokens of messages]"}] }
Agent: TOOL CALL send_message(...)
MCP: { "content": [{"type": "text", "text": "Message sent"}] }
```

**Cost**: 50,452 tokens

### After (Code Execution)

```
Agent: TOOL CALL execute_code(code: "
  result=$(anvillm/tools/anvilmcp/check_inbox.sh '82b93a8a')
  urgent=$(echo \"$result\" | grep -c 'URGENT')
  echo \"Found $urgent urgent messages\"
", language: "bash")
MCP: { "content": [{"type": "text", "text": "Found 3 urgent messages"}] }
```

**Cost**: 200 tokens (tool discovery) + 50 tokens (output) = 250 tokens

**Savings**: 99.5%

## Next Steps

- Read the [Migration Guide](./code-execution-migration-guide.md) for converting existing workflows
- Review [Security Documentation](./code-execution-security.md) for threat model and subprocess configuration
- Explore [Example Workflows](./code-execution-examples.md) for common patterns
