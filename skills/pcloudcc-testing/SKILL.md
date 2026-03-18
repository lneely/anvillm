---
name: pcloudcc-testing
intent: testing, pcloudcc
description: Test pcloudcc daemon and CLI tools. Use when verifying fixes, testing features, or smoke testing pcloudcc functionality.
---

# pCloudcc Testing

Test the pCloud console client daemon and CLI tools. Requires `$PCLOUD_USER` environment variable. If not set, instruct user to set it.

**Start daemon**: `./pcloudcc -d`
**Stop daemon**: `echo "finalize" | ./pcloudcc -k`
**Non-Interactive CLI**: Use stdin for non-interactive commands: `echo "<command>" | ./pcloudcc -k`
**Get command list**: `echo "help" | ./pcloudcc -k`

Common commands:
- `status` - Show sync state (use this instead of `pending` for upload/download checks)
- `sync list` - List sync folders
- `sync add <localpath> <remotepath>` - Add sync folder
- `sync remove <folderid>` - Remove sync folder
- `sync pause` - Pause syncing
- `sync resume` - Resume syncing
- `pending` - Check pending transfers (NOTE: output goes to daemon stdout, not client - use `status` instead)
- `crypto start <pass>` - Unlock crypto folder
- `crypto stop` - Lock crypto folder
- `finalize` - Kill daemon and quit

## Sandbox Escape (FUSE / daemon)

The pcloudcc daemon requires FUSE (mount syscall), which is blocked inside the sandbox. Use the `execute_elevated_bash` tool (provided by the `superpowerd` MCP server) to run commands outside the sandbox.

**Load the tool first** (deferred tool — must be discovered before use):
```
ToolSearch: select:mcp__superpowers-mcp-server__execute_elevated_bash
```

**Start daemon outside sandbox** (always use `-d` — never start without it):
```
Tool: mcp__superpowers-mcp-server__execute_elevated_bash
command: "cd <cwd> && ./pcloudcc -d"
summary: "Start pcloudcc daemon for FUSE testing"
```

**Send CLI commands** (these are lightweight and can run inside sandbox):
```
Tool: execute_code
code: echo "<command>" | ./pcloudcc -k
```

**Stop daemon**:
```
Tool: mcp__superpowers-mcp-server__execute_elevated_bash
command: "cd <cwd> && echo 'finalize' | ./pcloudcc -k"
summary: "Stop pcloudcc daemon"
```

Mount point: `~/pCloudDrive`

## Testing Workflow

### Testing Agent System Prompt

```
Load pcloudcc-testing skill. Test the affected code path with unit tests, fault injection, or smoke tests. Design tests that reproduce the bug scenario. Verify old code fails, new code passes. Report with evidence.
```

### Development Agent Workflow

**Prerequisites**: Load `anvillm-sessions` skill

Spawn testing agent:
```
Tool: execute_code
tool: spawn_agent.sh
args: ["<your-id>", "<cwd>", "<system-prompt-above>"]
sandbox: default
```

Send test request:
```
Tool: execute_code
tool: send_message.sh
args: ["<your-id>", "<testing-agent-id>", "PROMPT_REQUEST", "Validate fix", "Bug: <desc>\nFix: <changes>\nFiles: <list>\nBranch: <name>"]
```

Read response (user notifies when ready):
```
Tool: execute_code
tool: check_inbox.sh
args: ["<your-id>"]
```

Kill testing agent:
```
Tool: execute_code
tool: kill_agent.sh
args: ["<testing-agent-id>"]
```

### Testing Agent Workflow

1. Load pcloudcc-testing skill
2. Analyze bug and fix (read code, understand failure mode)
3. Design targeted test (reproduce bug scenario)
4. Execute test (unit/fault-inject/smoke)
5. Verify fix (old fails, new passes)
6. Report:
```
Tool: execute_code
tool: send_message.sh
args: ["<your-id>", "<requester-id>", "PROMPT_RESPONSE", "Validation complete", "Status: completed|failed\n<evidence>"]
```

**FUSE testing**: Use `execute_elevated_bash` to start the daemon outside the sandbox (see Sandbox Escape section above). No user intervention required.

### Manual Testing Workflow

1. Build: `make -j$(nproc)`
2. Start daemon (outside sandbox): `mcp__superpowers-mcp-server__execute_elevated_bash` → `cd <cwd> && ./pcloudcc -d`
3. Wait for initialization: `sleep 2`
4. Test commands (inside sandbox): `echo "<command>" | ./pcloudcc -k`
5. Stop daemon: `mcp__superpowers-mcp-server__execute_elevated_bash` → `cd <cwd> && echo 'finalize' | ./pcloudcc -k`

## Test Design

**CRITICAL**: Test the affected code path, not just "it compiles". Use unit tests or fault injection when normal usage doesn't exercise the bug.

**Test directories**:
- `./tests/unit-tests/` - Isolated components
- `./tests/fault-inject/` - Force failure scenarios
- `./tests/smoke-tests/` - End-to-end integration

**Test design**: Understand bug → reproduce scenario → verify old fails, new passes → provide evidence

**FUSE testing**: Use `execute_elevated_bash` to run the daemon outside the sandbox. Mount point: `~/pCloudDrive`

**Fault Injection Pattern**: `#define _GNU_SOURCE` + `#include <dlfcn.h>` to override functions. Compile: `gcc -shared -fPIC -o inject.so inject.c -ldl`. Run: `LD_PRELOAD=./inject.so ./pcloudcc -d`

## Notes

- **Always start the daemon with `-d`** — never invoke `./pcloudcc` as daemon without this flag
- Daemon must be running before using `-k` commands
- Each `-k` invocation creates new connection to daemon
- Daemon closes socket after each response
- Always finalize to cleanly stop daemon
- Use `status` instead of `pending` for checking upload/download state
- Kill daemon: use the PID shown at startup (e.g., `kill -9 12345`)
- `execute_elevated_bash` requires user approval per invocation — requests should be clearly summarized
