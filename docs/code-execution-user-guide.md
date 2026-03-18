# Code Execution User Guide

## Overview

`execute_code` is the only MCP tool provided by `anvilmcp`. It runs bash scripts in an isolated subprocess sandboxed via landrun. All AnviLLM functionality is accessed through 9P and helper scripts within these subprocesses.

## Tool Discovery

List available helper scripts:

```bash
9p ls anvillm/tools/
```

Read a script to see its usage:

```bash
9p read anvillm/tools/check_inbox.sh
```

Invoke a script:

```bash
bash <(9p read anvillm/tools/check_inbox.sh)
```

## Common Patterns

### Messaging

```bash
# Check inbox
bash <(9p read anvillm/tools/check_inbox.sh)

# Send message
bash <(9p read anvillm/tools/send_message.sh) \
  --to user --type PROMPT_RESPONSE --subject "Done" --body "Complete"
```

### Session Management

```bash
9p read anvillm/list
9p read anvillm/$ID/state
```

### Beads

```bash
9p read anvillm/beads/list | jq '[.[] | select(.status == "open")]'
echo 'new "Task title" "Task description"' | 9p write anvillm/beads/ctl
```

### Polling

```bash
while true; do
  state=$(9p read anvillm/$ID/state)
  [ "$state" = "idle" ] && break
  sleep 5
done
```

### Filtering

Process data in the subprocess — only return summaries to the agent:

```bash
beads=$(9p read anvillm/beads/list)
count=$(echo "$beads" | jq '[.[] | select(.priority <= 2)] | length')
echo "Found $count high-priority beads"
```

## Subprocess Restrictions

Scripts run in a landrun sandbox with limited permissions:

- Read/write to the session's working directory
- Read from system paths (`/usr`, `/lib`, `/bin`)
- Access to 9P namespace
- Standard utilities: `jq`, `grep`, `sed`, `awk`, `9p`
- Network access per sandbox config

## Error Handling

```bash
if output=$(9p read anvillm/beads/list 2>&1); then
  echo "$output" | jq 'length'
else
  echo "Failed: $output" >&2
  exit 1
fi
```

## Further Reading

- [Code Execution Pattern](./code-execution-pattern.md) — design rationale
- [Security Documentation](./code-execution-security.md) — threat model and sandbox config
- [Example Workflows](./code-execution-examples.md) — common patterns
