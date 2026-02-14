# Acme API Quick Reference

Fast lookup for common operations.

**Note**: This reference uses plan9port `9p` commands. On native Plan 9, use direct file paths like `/mnt/acme/`.

## File Paths (plan9port)

```
acme/index                       # List all windows
acme/new/ctl                     # Create new window
acme/$winid/addr                 # Set/query addresses
acme/$winid/body                 # Full body content
acme/$winid/ctl                  # Control commands
acme/$winid/data                 # Read/write at address (advances)
acme/$winid/errors               # Write errors to +Errors
acme/$winid/event                # Event stream
acme/$winid/tag                  # Full tag content
acme/$winid/xdata                # Read at address (doesn't advance)
```

## Accessing Files

```bash
9p ls acme                       # List acme namespace
9p read acme/path                # Read a file
9p write acme/path               # Write to file (from stdin)
echo "data" | 9p write acme/path # Write specific data
```

## Control Commands

```
clean                  # Mark clean
dirty                  # Mark dirty
show                   # Show window
name <path>            # Set window name
get                    # Load from disk
put                    # Save to disk
del                    # Delete (must be clean)
dot=addr               # Set selection
addr=addr              # Set addr file
```

## Address Syntax

```
0                      # Start of file
$                      # End of file
.                      # Current selection
#42                    # Byte offset 42
42                     # Rune position 42
/pattern/              # Forward search
-/pattern/             # Backward search
+n                     # n runes forward
-n                     # n runes backward
0,$                    # Entire file
.,+#100                # Current to +100 bytes
/start/,/end/          # Range between patterns
```

## Event Structure

```
Bytes 0-3:   c1 (origin: E, F, K, M)
Bytes 4-7:   c2 (type: L, D, I, X, l, d, i, x)
Bytes 8-11:  q0 (start byte offset)
Bytes 12-15: q1 (end byte offset)
Bytes 16-19: flag (chorded, arg)
Byte 20:     nr (rune count)
Bytes 21+:   text (nr runes, UTF-8)
```

## Event Types

```
L/l    Look (Button-3 click)
D/d    Delete
I/i    Insert
X/x    Execute (Button-2 click)

Uppercase = initiated, needs response
Lowercase = completed/acknowledged
```

## Event Origins

```
E      Keyboard in body
F      File operation (Get, Put, Undo)
K      Keyboard in tag
M      Mouse
```

## Event Flags

```
0x1    (unused)
0x2    Chorded operation
0x4    Has argument (selected text)
0x8    (unused)
```

## Common Patterns (Shell)

### Create Window

```bash
winid=$(9p read acme/new/ctl)
```

### Set Window Name

```bash
echo "name /path/to/file" | 9p write acme/$winid/ctl
```

### Read All Text

```bash
9p read acme/$winid/body
```

### Replace All Text

```bash
echo "new content" | 9p write acme/$winid/body
```

### Read Selection

```bash
echo "." | 9p write acme/$winid/addr
9p read acme/$winid/xdata
```

### Replace at Address

```bash
echo "0,5" | 9p write acme/$winid/addr
echo "replacement" | 9p write acme/$winid/data
```

### Append Text

```bash
echo "\$" | 9p write acme/$winid/addr
echo "appended" | 9p write acme/$winid/data
```

### Event Loop

Event handling requires binary I/O (not easily done with `9p` command).
Use Python/Go/C with subprocess or 9P library. See event-protocol.md.

### Accept Event

```
write_event(fd, event)  # Same event
```

### Reject Event

```
event.c2 = lowercase(event.c2)
write_event(fd, event)
```

## Binary Reading (Little-Endian)

### C

```c
uint32_t read_uint32(int fd) {
    uint32_t val;
    read(fd, &val, 4);
    return val;  // Assumes little-endian arch
}
```

### Python

```python
import struct
val = struct.unpack('<I', data)[0]  # Little-endian uint32
```

### Go

```go
import "encoding/binary"
val := binary.LittleEndian.Uint32(buf[0:4])
```

## Common Errors

### "No such file"
- Window doesn't exist
- Wrong window ID

### "Permission denied"
- Opening read-only file for write
- Opening event without read-write

### "Bad address"
- Invalid address syntax
- Pattern not found
- Position out of range

### Event blocks
- Forgot to acknowledge event
- Must write event back after read

### Wrong text length
- Reading nr bytes instead of nr runes
- UTF-8 character boundaries

## Shell Examples (9p command)

```bash
# Create window
winid=$(9p read acme/new/ctl)
echo "Created window $winid"

# Write to body
echo "content" | 9p write acme/$winid/body

# Read body
text=$(9p read acme/$winid/body)

# Send control commands
echo "name /tmp/file" | 9p write acme/$winid/ctl
echo "clean" | 9p write acme/$winid/ctl
echo "show" | 9p write acme/$winid/ctl

# Read index (list all windows)
9p read acme/index

# Append to body
echo "\$" | 9p write acme/$winid/addr
echo "new line" | 9p write acme/$winid/data
```

## Debugging Tips

### Log Events

```python
event = read_event(fd)
print(f"c1={event.c1} c2={event.c2} q0={event.q0} "
      f"q1={event.q1} flag={event.flag} text={event.text!r}")
write_event(fd, event)
```

### Test Address

```bash
echo "0,10" | 9p write acme/$winid/addr
9p read acme/$winid/addr  # Should show "0 10" or similar
```

### Check Window Exists

```bash
9p read acme/index | grep "^$winid "
```

### Monitor Window

```bash
# Watch for changes
while true; do
    9p read acme/$winid/body
    sleep 1
done
```

## Language-Specific Notes

### Shell (plan9port)

- Use `9p read` and `9p write` for all operations
- Simple and works everywhere plan9port is installed
- Limited for binary event handling
- Perfect for window management and text operations

### Python

- Call `9p` via `subprocess.run()`
- Or use `py9p` library for native 9P client
- Use `struct` for binary event parsing
- Example: `subprocess.run(['9p', 'read', 'acme/new/ctl'], capture_output=True)`

### Go

- Use `9fans.net/go/plan9/client` for native 9P
- Or call `9p` command via `exec.Command()`
- Binary encoding via `encoding/binary`

### C

- Call `9p` via `popen()`
- Or use `libixp`/`lib9p` for direct 9P
- Watch endianness (little-endian for events)

## Performance Tips

1. **Keep files open**: Don't open/close for each operation
2. **Use xdata for inspection**: Avoids addr movement
3. **Batch writes**: Combine multiple ctl commands
4. **Read efficiently**: Use addr+data for selective reads
5. **Event loop**: Single threaded, acknowledge promptly

## Standard Window Setup

```bash
# Typical window initialization
winid=$(9p read acme/new/ctl)
echo "name /path/to/file" | 9p write acme/$winid/ctl
echo "/path/to/file Del Get Put | fmt" | 9p write acme/$winid/tag
echo "Initial content" | 9p write acme/$winid/body
echo "clean" | 9p write acme/$winid/ctl
echo "show" | 9p write acme/$winid/ctl
```

## Resources

- `man 4 acme` - Official documentation
- Russ Cox's "Acme: A User Interface for Programmers"
- /sys/src/cmd/acme - Source code
- 9fans.net - Plan 9 resources
