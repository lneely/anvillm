# Migration Guide: Direct Tools to Code Execution

## Overview

This guide shows how to migrate from direct MCP tool calls to the code execution pattern. The migration reduces token consumption by 80-99% while enabling more powerful workflows.

## Key Differences

| Aspect | Direct Tools | Code Execution |
|--------|--------------|----------------|
| Tool loading | All upfront | Progressive discovery |
| Data flow | Through context | In subprocess |
| Control flow | Alternating calls | Loops, conditionals |
| Token cost | High (linear with data) | Low (constant) |
| Privacy | Data in context | Data isolated |

## Migration Patterns

### Pattern 1: Single Tool Call

**Before**:
```
Agent: TOOL CALL read_inbox(agent_id: "82b93a8a")
MCP: { "content": [{"type": "text", "text": "[Message 1]\n[Message 2]\n..."}] }
```

**After**:
```
execute_code with code: "bash <(9p read anvillm/tools/anvilmcp/read_inbox.sh) 82b93a8a"
```

**Token savings**: Minimal for single calls, but enables further optimization

### Pattern 2: Sequential Tool Calls

**Before**:
```
Agent: TOOL CALL read_inbox(agent_id: "agent-1")
MCP: { "content": [...] }  // 5,000 tokens
Agent: TOOL CALL read_inbox(agent_id: "agent-2")
MCP: { "content": [...] }  // 5,000 tokens
Agent: TOOL CALL read_inbox(agent_id: "agent-3")
MCP: { "content": [...] }  // 5,000 tokens
```

**Total**: 15,000 tokens

**After**:
```
execute_code with code: "
for agent_id in agent-1 agent-2 agent-3; do
  bash <(9p read anvillm/tools/anvilmcp/read_inbox.sh) \$agent_id | wc -l
done | awk '{sum+=\$1} END {print \"Total messages:\", sum}'
"
```

**Output**: "Total messages: 47" (~10 tokens)

**Token savings**: 99.9% (10 tokens vs 15,000 tokens)

### Pattern 3: Filtering Large Results

**Before**:
```
Agent: TOOL CALL get_spreadsheet(sheet_id: "abc123")
MCP: { "content": [...] }  // 200,000 tokens (10,000 rows)
Agent: [Processes in context, filters to 47 pending orders]
```

**Total**: 200,000+ tokens

**After**:
```
execute_code with code: "
rows=\$(9p read anvillm/sheets/abc123)
pending=\$(echo \"\$rows\" | jq '[.[] | select(.Status == \"pending\")]')
count=\$(echo \"\$pending\" | jq 'length')
total=\$(echo \"\$rows\" | jq 'length')
echo \"Found \$count pending orders out of \$total total\"
"
```

**Output**: "Found 47 pending orders out of 10000 total" (~15 tokens)

**Token savings**: 99.99% (15 tokens vs 200,000 tokens)

### Pattern 4: Polling Loops

**Before**:
```
Agent: TOOL CALL check_status(job_id: "job-123")
MCP: { "status": "running" }
Agent: [Waits, then calls again]
Agent: TOOL CALL check_status(job_id: "job-123")
MCP: { "status": "running" }
[Repeats 10 times]
```

**Total**: ~5,000 tokens (10 calls × 500 tokens)

**After**:
```
execute_code with code: "
job_id=job-123
status=running
attempts=0

while [ \"\$status\" = \"running\" ] && [ \$attempts -lt 10 ]; do
  status=\$(9p read anvillm/jobs/\$job_id/status)
  if [ \"\$status\" = \"running\" ]; then
    sleep 5
  fi
  attempts=\$((attempts + 1))
done

echo \"Job \$job_id status: \$status (checked \$attempts times)\"
"
```

**Output**: "Job job-123 status: completed (checked 7 times)" (~15 tokens)

**Token savings**: 99.7% (15 tokens vs 5,000 tokens)

### Pattern 5: Multi-Step Data Pipeline

**Before**:
```
Agent: TOOL CALL read_inbox(agent_id: "82b93a8a")
MCP: { "content": [...] }  // 10,000 tokens
Agent: [Identifies 5 urgent messages]
Agent: TOOL CALL send_message(to: "manager", body: "5 urgent messages")
MCP: { "content": "Message sent" }
Agent: TOOL CALL set_state(agent_id: "82b93a8a", state: "idle")
MCP: { "content": "State updated" }
```

**Total**: ~11,000 tokens

**After**:
```
execute_code with code: "
inbox=\$(bash <(9p read anvillm/tools/anvilmcp/read_inbox.sh) 82b93a8a)
urgent_count=\$(echo \"\$inbox\" | grep -c URGENT || echo 0)

if [ \$urgent_count -gt 0 ]; then
  bash <(9p read anvillm/tools/anvilmcp/send_message.sh) 82b93a8a manager NOTIFICATION 'Urgent messages' \"\$urgent_count urgent messages in inbox\"
fi

bash <(9p read anvillm/tools/anvilmcp/set_state.sh) 82b93a8a idle
echo \"Processed inbox: \$urgent_count urgent messages\"
"
```

**Output**: "Processed inbox: 5 urgent messages" (~10 tokens)

**Token savings**: 99.9% (10 tokens vs 11,000 tokens)

## Tool-Specific Migrations

### read_inbox

**Before**:
```
TOOL CALL read_inbox(agent_id: "82b93a8a")
```

**After**:
```
execute_code with code: "bash <(9p read anvillm/tools/anvilmcp/read_inbox.sh) 82b93a8a"
```

**Or use 9P directly**:
```
execute_code with code: "9p read anvillm/inbox/82b93a8a/json"
```

### send_message

**Before**:
```
TOOL CALL send_message(from: "agent-1", to: "agent-2", type: "PROMPT_REQUEST", subject: "Task", body: "Do X")
```

**After**:
```bash
bash <(9p read anvillm/tools/anvilmcp/send_message.sh) agent-1 agent-2 PROMPT_REQUEST 'Task' 'Do X'
```

### list_sessions

**Before**:
```
TOOL CALL list_sessions()
```

**After**:
```
execute_code with code: "bash <(9p read anvillm/tools/anvilmcp/list_sessions.sh)"
```

### set_state

**Before**:
```
TOOL CALL set_state(agent_id: "82b93a8a", state: "running")
```

**After**:
```
execute_code with code: "bash <(9p read anvillm/tools/anvilmcp/set_state.sh) 82b93a8a running"
```

### list_skills

**Before**:
```
TOOL CALL list_skills()
```

**After**:
```
execute_code with code: "bash <(9p read anvillm/tools/anvilmcp/list_skills.sh) | jq -r '.[].name' | paste -sd, -"
```

## Compatibility Notes

### When to Use Direct Tools

Direct tools are better for:

1. **Single operations**: Simple one-off calls
2. **Small data**: Results under 1,000 tokens
3. **Simple logic**: No conditionals or loops needed
4. **Debugging**: When you need to see intermediate results

### When to Use Code Execution

Code execution is better for:

1. **Multiple tool calls**: Any workflow with 2+ tool calls
2. **Large data**: Any operation returning >1,000 tokens
3. **Filtering/aggregation**: Processing data to extract summaries
4. **Loops**: Polling, batch processing, retries
5. **Privacy**: Operations involving PII or sensitive data

## Migration Checklist

- [ ] Identify workflows with multiple tool calls
- [ ] Identify operations with large data returns
- [ ] Convert sequential calls to loops
- [ ] Add filtering/aggregation in subprocess
- [ ] Return summaries instead of raw data
- [ ] Add error handling (try/catch)
- [ ] Test in isolated subprocess
- [ ] Measure token savings
- [ ] Update agent prompts with examples

## Common Pitfalls

### Pitfall 1: Returning Raw Data

**Bad**:
```bash
result=$(bash <(9p read anvillm/tools/anvilmcp/read_inbox.sh) 82b93a8a)
echo "$result"  # Could be 50,000 tokens
```

**Good**:
```bash
result=$(bash <(9p read anvillm/tools/anvilmcp/read_inbox.sh) 82b93a8a)
count=$(echo "$result" | wc -l)
echo "Inbox has $count messages"  # ~10 tokens
```

### Pitfall 2: Not Using Loops

**Bad**:
```bash
result1=$(bash <(9p read anvillm/tools/anvilmcp/read_inbox.sh) agent-1)
echo "$result1"
result2=$(bash <(9p read anvillm/tools/anvilmcp/read_inbox.sh) agent-2)
echo "$result2"
result3=$(bash <(9p read anvillm/tools/anvilmcp/read_inbox.sh) agent-3)
echo "$result3"
```

**Good**:
```bash
for agent_id in agent-1 agent-2 agent-3; do
  result=$(bash <(9p read anvillm/tools/anvilmcp/read_inbox.sh) "$agent_id")
  count=$(echo "$result" | wc -l)
  echo "$agent_id: $count messages"
done
```

### Pitfall 3: Ignoring Errors

**Bad**:
```bash
result=$(bash <(9p read anvillm/tools/anvilmcp/read_inbox.sh) invalid-id)
echo "$result"  # May fail silently
```

**Good**:
```bash
if result=$(bash <(9p read anvillm/tools/anvilmcp/read_inbox.sh) invalid-id 2>&1); then
  echo "$result"
else
  echo "Failed to read inbox: $result" >&2
fi
```

### Pitfall 4: Re-reading Tool Definitions

**Bad**:
```bash
for i in {1..10}; do
  tool=$(9p read anvillm/tools/anvilmcp/read_inbox.sh)
  # Parse and use...
done
```

**Good**:
```bash
# Call tool directly each time
for i in {1..10}; do
  bash <(9p read anvillm/tools/anvilmcp/read_inbox.sh) 82b93a8a
done
```

## Performance Comparison

| Workflow | Direct Tools | Code Execution | Savings |
|----------|--------------|----------------|---------|
| Single small call | 500 tokens | 400 tokens | 20% |
| 3 sequential calls | 15,000 tokens | 150 tokens | 99% |
| Filter 10k rows | 200,000 tokens | 100 tokens | 99.95% |
| Poll 10 times | 5,000 tokens | 200 tokens | 96% |
| Multi-step pipeline | 50,000 tokens | 500 tokens | 99% |

## Rollout Strategy

### Phase 1: Pilot (Week 1)
- Migrate 1-2 simple workflows
- Measure token savings
- Identify issues

### Phase 2: Expand (Week 2-3)
- Migrate high-volume workflows
- Update agent prompts
- Train team on patterns

### Phase 3: Optimize (Week 4+)
- Refine implementations
- Add more examples
- Monitor metrics

## Support

For questions or issues:
- Review [User Guide](./code-execution-user-guide.md)
- Check [Security Documentation](./code-execution-security.md)
- See [Example Workflows](./code-execution-examples.md)

## Token Savings Calculator

Estimate your savings:

```
Current tokens = (tool_count × 90) + (data_size_kb × 250)
Code execution tokens = 30 + (summary_size_kb × 250)
Savings = (Current - Code execution) / Current × 100%
```

Example:
- 100 tools, 10 KB data
- Current: (100 × 90) + (10 × 250) = 11,500 tokens
- Code execution: 30 + (0.01 × 250) = 33 tokens
- Savings: 99.7%
