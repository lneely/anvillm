# Security Considerations

## 9P Server Socket

The anvilsrv 9P server creates a Unix domain socket at `$NAMESPACE/agent` (typically `/tmp/ns.$USER/agent`).

**Important Security Notes:**

1. **No Authentication**: The 9P socket has **no authentication mechanism**. Any local user who can access the socket can:
   - List all running sessions
   - Send prompts to any session
   - Read session output
   - Control session lifecycle (stop, restart, kill)
   - Modify session aliases and context

2. **Access Control**: Security relies entirely on Unix file permissions:
   - The socket is created in the user's namespace directory (`/tmp/ns.$USER/`)
   - Namespace directory permissions should be `0700` (user-only access)
   - On multi-user systems, verify namespace directory permissions

3. **Recommendations**:
   - **Single-user systems**: Default configuration is secure
   - **Multi-user systems**:
     - Verify `/tmp/ns.$USER/` is mode 0700
     - Do not share session IDs with untrusted users
     - Be aware that root can always access user sessions
   - **Shared systems**: Consider additional isolation (containers, VMs, separate machines)

4. **What This Means**:
   - Anyone with access to the socket can interact with your AI sessions
   - Session data (prompts, responses) can be read by socket users
   - Sessions can be terminated or manipulated by socket users

5. **Mitigation**:
   ```sh
   # Verify namespace directory permissions
   ls -ld $NAMESPACE
   # Should show: drwx------ (0700)

   # If not, fix permissions:
   chmod 700 $NAMESPACE
   ```

## PID File

The PID file is stored at `$NAMESPACE/anvilsrv.pid` to prevent symlink attacks. Older versions used `/tmp/anvilsrv.pid` which was vulnerable.

## Path Validation

The Assist client validates all user-provided paths before creating sessions:
- Paths are cleaned and normalized with `filepath.Clean()`
- Paths must be absolute (or converted to absolute)
- Paths must exist and be directories
- This prevents path traversal attacks

## Process Signals

When checking if a process is running via `syscall.Kill(pid, 0)`:
- `ESRCH` error means process doesn't exist (triggers refresh)
- `EPERM` error means process exists but we can't signal it (no action)
- This prevents false positives when checking backend processes

## Tmux Sessions

Backend sessions run in tmux windows with:
- One persistent tmux session per backend type
- Session windows named by session ID
- FIFO pipes for output capture at `/tmp/tmux-{session}-{window}.fifo`

**Security Notes**:
- Tmux sessions are accessible to the user running anvilsrv
- FIFO files in /tmp could theoretically be accessed by other local users
- Backend processes inherit anvilsrv's environment and permissions

## Future Improvements

Potential security enhancements for consideration:
1. Add authentication to 9P protocol (shared secret, capabilities)
2. Per-session access control lists
3. Audit logging of 9P operations
4. Encrypted transport for 9P (though Unix sockets are local-only)
5. Namespace isolation via Linux namespaces or FreeBSD jails
