# Testing Code Execution Pattern Adoption

## Overview

This document describes how to verify that agents are using the code execution pattern instead of direct tool calls, and how to measure token reduction.

## Test Script

Run `./test-code-execution.sh` to automatically test agent adoption of the code execution pattern.

The script:
1. Creates a test agent with Taskmaster template
2. Sends queries that should trigger execute_code usage
3. Verifies tool usage patterns
4. Cleans up test agent

## Manual Testing

### Test Case 1: Data Filtering

**Query**: "Find all beads that haven't been updated in 30+ days and are still open. Show count by priority."

**Expected behavior**:
- Agent uses `execute_code` with bash
- Loads beads once with `beads.list()`
- Filters in subprocess using `.filter()` and date comparison
- Aggregates by priority in subprocess
- Returns only summary statistics to context

**Anti-pattern** (what NOT to do):
- Multiple `beads.list()` calls
- Loading all bead data into context
- Using separate tool calls for each filter condition

### Test Case 2: Batch Operations

**Query**: "Update all priority 1 beads to priority 2. Show progress."

**Expected behavior**:
- Agent uses `execute_code` with bash
- Queries beads once with `beads.query({ priority: 1 })`
- Loops through results in subprocess
- Updates each bead with `beads.update()`
- Reports progress periodically (e.g., every 10 items)
- Returns final summary to context

**Anti-pattern**:
- Alternating between tool calls and model responses for each update
- Loading full bead details into context for each item

### Test Case 3: Multi-Source Joins

**Query**: "Show all open beads that have related KB entries. Include KB reference count."

**Expected behavior**:
- Agent uses `execute_code` with bash
- Loads beads and KB entries in parallel
- Joins data in subprocess
- Filters to only beads with KB references
- Returns enriched summary to context

**Anti-pattern**:
- Sequential tool calls for each bead
- Loading all KB content into context
- Performing join logic through model reasoning

## Measuring Token Reduction

### Baseline (Direct Tool Calls)

For a query that processes 1000 beads:
- Tool definitions: ~150k tokens (all tools loaded upfront)
- Tool results: ~500k tokens (all bead data in context)
- Total: ~650k tokens

### With Code Execution

For the same query:
- Tool definitions: ~2k tokens (only beads.sh loaded)
- Code execution: ~1k tokens (bash code)
- Results: ~500 tokens (filtered summary)
- Total: ~3.5k tokens

**Reduction**: 99.5% (650k → 3.5k tokens)

### Measuring in Practice

1. **Enable token tracking** in agent stats:
   ```bash
   9p read anvillm/<agent-id>/stats
   ```

2. **Compare before/after**:
   - Run same query with direct tool calls
   - Run same query with code execution
   - Calculate reduction percentage

3. **Look for indicators**:
   - Fewer tool calls in agent logs
   - Smaller context window usage
   - Faster response times
   - Lower API costs

## Verification Checklist

- [ ] Agent uses `execute_code` for data transformations
- [ ] Agent loads tool definitions on-demand (progressive discovery)
- [ ] Agent filters data in subprocess before returning to context
- [ ] Agent uses loops in subprocess instead of alternating tool calls
- [ ] Token usage reduced by 80-99% compared to direct tool calls
- [ ] Response times improved
- [ ] Agent behavior is correct (no regressions)

## Common Issues

### Agent still using direct tool calls

**Cause**: Agent prompt doesn't emphasize code execution pattern strongly enough.

**Fix**: Add more prominent code execution examples to agent context. Show specific use cases relevant to agent's role.

### Agent loads all tools upfront

**Cause**: Agent doesn't understand progressive discovery workflow.

**Fix**: Add explicit instructions to list `anvillm/tools/anvilmcp/` first, then read only needed tool files.

### Agent passes large datasets through context

**Cause**: Agent doesn't filter data in subprocess.

**Fix**: Add examples showing filtering, aggregation, and summarization in subprocess before returning results.

## Success Criteria

An agent successfully adopts the code execution pattern when:

1. **Token usage** is reduced by 80-99% for data-heavy operations
2. **Tool calls** are consolidated (1-2 execute_code calls instead of 10+ direct tool calls)
3. **Context efficiency** is improved (only summaries and filtered results in context)
4. **Correctness** is maintained (same or better results than direct tool calls)
5. **Response time** is improved (fewer round-trips to model)
