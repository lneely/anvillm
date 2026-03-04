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

## Testing Workflow

### Testing Agent System Prompt

```
Load pcloudcc-testing skill. Test the affected code path with unit tests, fault injection, or smoke tests. Design tests that reproduce the bug scenario. Verify old code fails, new code passes. Report with evidence.
```

### Development Agent Workflow

**Prerequisites**: Load `anvillm-sessions` and `anvillm-communication` skills

**Spawn testing agent**: `bash <(9p read agent/tools/agents/spawn_agent.sh) <your-id> "<system-prompt-above>"`

**Send test request**: `bash <(9p read agent/tools/messaging/send_message.sh) <your-id> <testing-agent-id> PROMPT_REQUEST "Validate fix" "Bug: <desc>\nFix: <changes>\nFiles: <list>\nBranch: <name>"`

**Read response** (user notifies when ready): `bash <(9p read agent/tools/messaging/read_inbox.sh) <your-id>`

**Kill testing agent**: `bash <(9p read agent/tools/agents/kill_agent.sh) <testing-agent-id>`

**Note**: `spawn_agent.sh` is an exception — use the Bash tool (`execute_bash`), NOT `execute_code`. All other commands in this section use `execute_code` as normal.

### Testing Agent Workflow

1. Load pcloudcc-testing skill
2. Analyze bug and fix (read code, understand failure mode)
3. Design targeted test (reproduce bug scenario)
4. Execute test (unit/fault-inject/smoke)
5. Verify fix (old fails, new passes)
6. Report: `bash <(9p read agent/tools/messaging/send_message.sh) <your-id> <requester-id> PROMPT_RESPONSE "Validation complete" "Status: completed|failed\n<evidence>"`

**FUSE testing**: Inform user daemon must start outside sandbox, wait for acknowledgement

### Manual Testing Workflow

1. Build: `make -j$(nproc)`
2. Start daemon: `./pcloudcc -d`
3. Wait for initialization: `sleep 2`
4. Test commands: `echo "<command>" | ./pcloudcc -k`
5. Stop daemon: `echo "finalize" | ./pcloudcc -k`

## Test Design

**CRITICAL**: Test the affected code path, not just "it compiles". Use unit tests or fault injection when normal usage doesn't exercise the bug.

**Test directories**:
- `./tests/unit-tests/` - Isolated components
- `./tests/fault-inject/` - Force failure scenarios
- `./tests/smoke-tests/` - End-to-end integration

**Test design**: Understand bug → reproduce scenario → verify old fails, new passes → provide evidence

**FUSE testing**: Daemon must start outside sandbox (mount syscall blocked). Inform user, wait for acknowledgement. Mount: `~/pCloudDrive`

**Fault Injection Pattern**: `#define _GNU_SOURCE` + `#include <dlfcn.h>` to override functions. Compile: `gcc -shared -fPIC -o inject.so inject.c -ldl`. Run: `LD_PRELOAD=./inject.so ./pcloudcc -d`

## Notes

- Daemon must be running before using `-k` commands
- Each `-k` invocation creates new connection to daemon
- Daemon closes socket after each response
- Always finalize to cleanly stop daemon
- Use `status` instead of `pending` for checking upload/download state
- Kill daemon: use the PID shown at startup (e.g., `kill -9 12345`)
