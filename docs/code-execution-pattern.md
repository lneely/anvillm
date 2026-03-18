# Code Execution Pattern

## Overview

`execute_code` is the single MCP tool exposed by `anvilmcp`. It executes bash scripts in an isolated subprocess sandboxed via landrun. All AnviLLM functionality — messaging, sessions, beads, state — is accessed through 9P and helper scripts within these subprocesses.

## Tool Discovery

Discover available helper scripts on-demand via 9P:

```bash
9p ls anvillm/tools/
```

Read and invoke a specific script:

```bash
bash <(9p read anvillm/tools/check_inbox.sh)
```

## Examples

### Messaging

```bash
bash <(9p read anvillm/tools/send_message.sh) \
  --to user --type PROMPT_RESPONSE --subject "Done" --body "Task complete"
```

### Beads

```bash
echo 'new "Implement login" "Add JWT auth"' | 9p write anvillm/beads/ctl
9p read anvillm/beads/ready | jq -r '.[] | "\(.id): \(.title)"'
```

### Polling

```bash
while true; do
  state=$(9p read anvillm/$ID/state)
  [ "$state" = "idle" ] && break
  sleep 5
done
echo "Session $ID is idle"
```

### Filtering

```bash
9p read anvillm/beads/list | jq '[.[] | select(.status == "open" and .priority <= 2)]'
```

## Why One Tool

A single `execute_code` tool means:
- No upfront tool-definition loading (saves tokens)
- Data stays in the subprocess (not in context)
- Loops, conditionals, and pipes work naturally
- Progressive discovery — only read what you need
