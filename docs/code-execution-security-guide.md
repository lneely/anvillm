# Code Execution Security Guide

## Overview

The execute_code tool runs bash scripts in isolated subprocesses using landrun (Landlock LSM). This document covers the security model, threat analysis, and configuration.

## Security Architecture

```
Agent (Model)
    ↓ generates bash code
anvilmcp (validates)
    ↓ spawns subprocess
landrun (Landlock sandbox)
    ↓ restricts filesystem/network
bash subprocess
    ↓ executes code
    ↓ accesses 9P via socket
anvilsrv (9P server)
```

## Isolation Mechanisms

### 1. Landlock LSM (Kernel-Enforced)

Landlock provides mandatory access control at the kernel level. Restrictions cannot be bypassed by the subprocess.

**Filesystem restrictions**:
- rwx: `~/.cache/anvillm/exec/<workspace>` (workspace only)
- ro: `/usr`, `/lib`, `/lib64`, `/bin`, `/etc/alternatives`, `/etc/ld.so.cache` (system binaries)
- rw: `$NAMESPACE` (9P socket)
- Everything else: denied

**Network**: Not explicitly restricted (relies on outer landrun if needed)

**Nested sandboxes**: Inner landrun can only add MORE restrictions, never relax them. Restrictions stack.

### 2. Workspace Isolation

Each execution gets a fresh temporary directory:
- Path: `~/.cache/anvillm/exec/anvilmcp-<random>`
- Permissions: 0700 (user-only)
- Lifecycle: Created before execution, deleted after
- Working directory: Set to workspace

Scripts can only write to their workspace. All other filesystem locations are read-only or denied.

### 3. Resource Limits

**Timeout**: 30 seconds default (configurable per call)
- Enforced via context.WithTimeout
- Subprocess killed on timeout
- Logged as security event

**Output size**: 10 MB maximum
- Enforced via limitedWriter
- Prevents memory exhaustion
- Returns error if exceeded

**Concurrency**: 3 simultaneous executions maximum
- Enforced via semaphore
- Prevents resource exhaustion
- Blocks until slot available

### 4. Code Validation

Dangerous patterns rejected before execution:
- `rm -rf /` - Filesystem destruction
- `:(){ :|:& };:` - Fork bomb
- `mkfs` - Filesystem formatting
- `dd if=/dev/zero` - Device writes
- `chmod 777` - Permission escalation
- `curl http` / `wget http` - Network access
- `> /dev/` - Device writes
- `exec(` / `eval(` - Code injection

Validation is basic pattern matching. Not comprehensive - relies on sandbox for enforcement.

## Threat Model

### Threat 1: Malicious Code Injection

**Scenario**: Agent generates malicious bash code

**Risk**: LOW

**Mitigations**:
1. Agent-generated code (not user input)
2. Landlock restricts filesystem access
3. Workspace isolation
4. Code validation (basic)
5. Timeout enforcement

**Residual risk**: Agent could generate code that consumes CPU within timeout, or writes garbage to workspace. Acceptable for trusted agents.

### Threat 2: Sandbox Escape

**Scenario**: Subprocess breaks out of landrun restrictions

**Risk**: LOW

**Mitigations**:
1. Landlock is kernel-enforced (not userspace)
2. No known Landlock bypasses (as of kernel 5.13+)
3. Nested sandboxes only add restrictions
4. No setuid binaries in allowed paths

**Residual risk**: Kernel vulnerability in Landlock. Mitigated by keeping kernel updated.

### Threat 3: Resource Exhaustion

**Scenario**: Code consumes excessive CPU, memory, or disk

**Risk**: MEDIUM

**Mitigations**:
1. Timeout (30s default)
2. Output size limit (10 MB)
3. Concurrency limit (3 max)
4. Workspace cleanup on completion

**Residual risk**: CPU exhaustion within timeout window. Could add ulimit for CPU/memory if needed.

### Threat 4: Data Exfiltration

**Scenario**: Sensitive data leaked from subprocess

**Risk**: LOW

**Mitigations**:
1. Data stays in subprocess (not in context)
2. Only summary/results returned
3. No network access (unless explicitly granted)
4. 9P socket access controlled by outer sandbox

**Residual risk**: Data could be written to workspace, but workspace is deleted. Data could be sent via 9P to other agents.

### Threat 5: 9P Socket Access

**Scenario**: Subprocess abuses 9P access to read/write unauthorized data

**Risk**: MEDIUM

**Mitigations**:
1. 9P socket permissions (Unix socket, user-only)
2. anvilsrv enforces access control
3. Observability via event stream, mailbox archives, and debug logs

**Residual risk**: If subprocess has 9P access, it has the same capabilities as any 9P client. This is by design.

**Mitigation**: Run locally on a trusted network. On a single-user system, the only processes with socket access are ones the user launched. Do not expose the 9P mount over the network, or at minimum, implement access control (e.g., firewall).

### Threat 6: Path Traversal

**Scenario**: Agent reads tools outside mcptools directory

**Risk**: LOW

**Mitigations**:
1. Filename validation (reject `..` and `/`)
2. 9P path normalization
3. Direct file read from known directory

**Residual risk**: None - path traversal blocked at multiple layers.

## Security Properties

- Data isolation (subprocess, not in model context)
- Privacy preservation (PII doesn't enter context)
- Explicit control over data flow
- Workspace isolation
- 9P socket access control
- Process isolation via landrun (Landlock)

## Configuration

### Sandbox Selection

By default (when the `sandbox` argument is not specified), `execute_code` uses the restricted `anvilmcp` sandbox configuration. If this fails (e.g., due to kernel or permission constraints), it falls back to the `default` sandbox — the same configuration used by the outer landrun invocation. This compromise provides "just works" behavior and avoids surprises, while still applying the tighter sandbox when possible.

For higher-risk scenarios (e.g., running untrusted scripts, processing sensitive data), the `anvilmcp` sandbox should be explicitly specified to ensure the tighter restrictions are enforced — it uses a temporary workspace, minimal filesystem access, and a restricted set of environment variables.

Custom sandbox configurations can be created by adding a YAML file to the `sandbox/` subdirectory in the anvillm config directory (`~/.config/anvillm/sandbox/`). These can be as permissive or restricted as desired.

### Adjusting Timeout

Default: 30 seconds

Increase for long-running operations:
```
execute_code with code: "..." timeout: 120
```

Decrease for quick operations:
```
execute_code with code: "..." timeout: 5
```

### Adjusting Output Limit

Default: 10 MB

To change, edit `cmd/anvilmcp/execute.go`:
```go
limitedWriter := &limitedWriter{w: &outputBuf, limit: 50 * 1024 * 1024} // 50 MB
```

### Adjusting Concurrency

Default: 3 simultaneous executions

To change, edit `cmd/anvilmcp/main.go`:
```go
executionSemaphore = make(chan struct{}, 10) // 10 concurrent
```

### Adding Filesystem Paths

To grant access to additional paths, edit `cmd/anvilmcp/execute.go`:
```go
cmd = exec.CommandContext(ctx, landrunPath,
    "-rwx", workDir,
    "-ro", "/usr",
    "-ro", "/additional/path", // Add here
    // ...
)
```

**Warning**: Only add paths that are safe for agent access. Avoid home directory, /etc, /var.

### Disabling Network Access

Network is not explicitly restricted by default. To block:

1. Use outer landrun with network restrictions
2. Or add iptables rules for the user
3. Or use network namespaces

Example outer landrun:
```bash
landrun -no-net -- anvilmcp
```

## Monitoring

### Execution Logs

Location: `~/.cache/anvillm/logs/anvilmcp-exec.log`

Fields:
- Timestamp
- Code hash (SHA256)
- Language
- Duration
- Success/failure
- Output size
- Error message

### Security Events

Location: `~/.cache/anvillm/logs/anvilmcp-security.log`

Events:
- Validation failures (dangerous patterns)
- Timeouts
- Output size limit exceeded
- Sandbox errors

### Monitoring Commands

Check recent executions:
```bash
tail -f ~/.cache/anvillm/logs/anvilmcp-exec.log
```

Check security events:
```bash
grep -i error ~/.cache/anvillm/logs/anvilmcp-security.log
```

Count executions by status:
```bash
jq -r '.Success' ~/.cache/anvillm/logs/anvilmcp-exec.log | sort | uniq -c
```

## Incident Response

### Suspicious Activity

If you observe suspicious code execution:

1. Check security logs for validation failures
2. Review agent context/prompts for anomalies
3. Check workspace contents (if not yet deleted)
4. Review 9P access logs in anvilsrv

### Sandbox Escape Attempt

If you suspect sandbox escape:

1. Kill all anvilmcp processes immediately
2. Check kernel logs: `dmesg | grep landlock`
3. Review Landlock version: `uname -r` (need 5.13+)
4. Update kernel if outdated
5. Report to security team

### Resource Exhaustion

If system becomes unresponsive:

1. Kill anvilmcp: `pkill -9 anvilmcp`
2. Check running subprocesses: `ps aux | grep bash`
3. Review execution logs for long-running operations
4. Adjust timeout/concurrency limits
5. Consider adding ulimit restrictions

## Best Practices

### For Agents

1. **Minimize code complexity**: Simple scripts are easier to validate
2. **Use timeouts**: Don't rely on default, specify appropriate timeout
3. **Handle errors**: Check exit codes, parse stderr
4. **Limit output**: Filter/aggregate in subprocess, return summaries
5. **Clean up**: Don't leave large files in workspace (auto-deleted anyway)

### For Operators

1. **Monitor logs**: Regular review of execution and security logs
2. **Update kernel**: Keep Landlock up-to-date
3. **Tune limits**: Adjust timeout/concurrency based on workload
4. **Audit access**: Review 9P access patterns
5. **Test sandbox**: Periodically verify restrictions are enforced

### For Developers

1. **Validate inputs**: Don't trust agent-generated code blindly
2. **Fail closed**: Reject on validation error, don't execute
3. **Log everything**: Execution attempts, failures, security events
4. **Test escapes**: Regularly test sandbox with malicious code
5. **Update patterns**: Add new dangerous patterns as discovered

## Testing Sandbox

Verify restrictions are enforced:

### Test 1: Filesystem Write Outside Workspace
```bash
execute_code with code: "echo test > /tmp/escape.txt"
# Expected: Permission denied
```

### Test 2: Read Sensitive Files
```bash
execute_code with code: "cat /etc/shadow"
# Expected: Permission denied
```

### Test 3: Network Access
```bash
execute_code with code: "curl http://example.com"
# Expected: Command not found or network unreachable
```

### Test 4: Fork Bomb
```bash
execute_code with code: ":(){ :|:& };:"
# Expected: Validation error (rejected before execution)
```

### Test 5: Timeout
```bash
execute_code with code: "sleep 60" timeout: 5
# Expected: Timeout after 5 seconds
```

### Test 6: Output Limit
```bash
execute_code with code: "dd if=/dev/zero bs=1M count=20"
# Expected: Output size limit exceeded
```

All tests should fail safely without compromising the system.

## References

- Landlock documentation: https://docs.kernel.org/userspace-api/landlock.html
- landrun: https://github.com/zouuup/landrun
- Anthropic code execution pattern: https://www.anthropic.com/engineering/code-execution-with-mcp
- User guide: ./code-execution-user-guide.md
