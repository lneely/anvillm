# State Machine Verification

## States

- `starting` - Session is initializing
- `idle` - Session is ready to accept work
- `running` - Session is actively processing a prompt
- `stopped` - Session backend has been stopped (can be restarted)
- `error` - Session encountered an error
- `exited` - Session has been closed (terminal state)

## Allowed Transitions (from transitionToLocked)

| From State | To State | Condition | Notes |
|------------|----------|-----------|-------|
| `starting` | `idle` | Always | Normal startup completion |
| `starting` | `error` | Always | Startup failure |
| `idle` | `running` | `reserved == true` | Must reserve before running |
| `running` | `idle` | Always | Normal completion |
| `running` | `error` | Always | Execution error |
| `*` (any) | `stopped` | Always | Stop from any state |
| `*` (any) | `exited` | Always | Close from any state |
| `stopped` | `starting` | Always | Restart after stop |

## Actual Transitions in Code

### session.go

| Location | Entry State | Exit State | Context |
|----------|-------------|------------|---------|
| `reader()` line 352 | `*` (any) | `stopped` | Reader EOF/error, not superseded/exited |
| `Send()` line 550 | `idle` | `running` | Before sending prompt |
| `Send()` line 564 | `running` | `idle` | sendLiteral failed |
| `Send()` line 573 | `running` | `idle` | sendKeys failed |
| `Send()` line 579 | `running` | `idle` | waitForComplete failed |
| `Send()` line 587 | `running` | `idle` | Normal completion |
| `SendAsync()` line 695 | `idle` | `running` | Before sending prompt |
| `SendAsync()` line 719 | `running` | `idle` | sendLiteral failed |
| `SendAsync()` line 733 | `running` | `error` | sendKeys failed (partial input) |
| `SendAsync()` line 744 | `running` | `error` | waitForComplete failed (goroutine) |
| `SendAsync()` line 748 | `running` | `idle` | Normal completion (goroutine) |
| `stopInternal()` line 841 | `*` (any) | `exited` | Window unexpectedly destroyed |
| `stopInternal()` line 848 | `*` (any) | `stopped` | Normal stop |
| `Restart()` line 1033 | `stopped` | `starting` | After restarting backend |
| `Restart()` line 1046 | `starting` | `error` | waitForReady failed |
| `Restart()` line 1050 | `starting` | `idle` | Restart successful |

### tmux.go

| Location | Entry State | Exit State | Context |
|----------|-------------|------------|---------|
| `New()` line 339 | `starting` | `error` | waitForReady failed |
| `New()` line 343 | `starting` | `idle` | Startup successful |

### server.go (p9)

| Location | Entry State | Exit State | Context |
|----------|-------------|------------|---------|
| `write()` line 621 | `*` (any) | `*` (user input) | Manual state write to `state` file |
| `write()` line 652 | `running` | `idle` | After writing to outbox |

### manager.go

| Location | Entry State | Exit State | Context |
|----------|-------------|------------|---------|
| `processInbox()` line 156 | `running` | `idle` | After processing inbox message |

## Missing Transitions

### Identified Issues

1. **error → idle**: No transition exists to recover from error state
   - Sessions stuck in error state cannot be recovered without restart
   - Should add: `error → idle` (manual recovery) or `error → starting` (auto-recovery)

2. **error → starting**: No transition exists to restart from error state
   - Must manually transition to stopped first, then restart
   - Should add: `error → starting` (direct restart from error)

3. **idle → stopped**: Not explicitly allowed but happens via `*` → `stopped`
   - This is correct (can stop idle sessions)

4. **idle → error**: No direct transition
   - This is correct (idle sessions don't error, only running sessions do)

5. **starting → stopped**: Not explicitly allowed but happens via `*` → `stopped`
   - This is correct (can stop during startup)

6. **starting → running**: No transition exists
   - This is correct (must go through idle first)

## Recommendations

### Critical

1. **Add error recovery transitions**:
   ```go
   case s.state == "error" && newState == "idle":
       // Manual recovery from error state
       s.state = newState
       s.reserved = false
       s.idleCond.Broadcast()
       return nil
   case s.state == "error" && newState == "starting":
       // Restart from error state
       s.state = newState
       return nil
   ```

2. **Document terminal states**: `exited` is terminal (no transitions out)

### Optional

1. **Add state transition logging**: Log all state transitions for debugging
2. **Add state transition metrics**: Track transition counts for monitoring
3. **Add state transition validation tests**: Unit tests for all valid/invalid transitions
