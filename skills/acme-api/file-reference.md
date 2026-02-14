# Acme File Reference

Complete reference for all files in the acme 9P interface.

**Note**: Examples use plan9port `9p` command syntax. On native Plan 9, use direct file paths like `/mnt/acme/`.

## Global Files

### acme/index

**Purpose**: Lists all current windows with metadata.

**Operations**:
- **Read**: Get list of all windows

**Format**: One line per window
```
winid nr minx miny maxx maxy tag-text
```

Fields:
- `winid`: Window ID number
- `nr`: Number of runes in tag (not including newline)
- `minx, miny, maxx, maxy`: Window position/size
- `tag-text`: Current tag content

**Usage**:
```bash
# List all windows
9p read acme/index

# Parse to get window IDs
9p read acme/index | awk '{print $1}'

# Use winid for operations on specific windows
```

### acme/new/ctl

**Purpose**: Create new windows.

**Operations**:
- **Read**: Creates a new window, returns its metadata

**Format**: Reading returns full ctl output with multiple fields
```
winid tagwidth bodywidth isdir ismodified taglines fontname bodylines ...
```
Example:
```
42          18           0           0           0        1803 /mnt/font/...
```

**Usage**:
```bash
# Create a new window and extract the window ID (first field)
winid=$(9p read acme/new/ctl | awk '{print $1}')
echo "Created window $winid"

# Use the window ID for further operations
echo "name /tmp/file" | 9p write acme/$winid/ctl
```

**Notes**:
- Each read creates a new window
- Don't read multiple times unless you want multiple windows
- New windows start with empty tag/body
- **IMPORTANT**: The ctl output contains many fields; extract just the first field for the window ID

## Per-Window Files

All files below are located in `acme/$winid/` where `$winid` is the window ID.

### ctl

**Purpose**: Control window and query state.

**Operations**:
- **Write**: Send control commands
- **Read**: Get window state (ID and tag width)

**Write Commands**:

| Command | Description |
|---------|-------------|
| `clean` | Mark window clean (no unsaved changes) |
| `dirty` | Mark window dirty (has unsaved changes) |
| `show` | Make window visible on screen |
| `hide` | Hide window (Plan 9 only) |
| `name path` | Set window name/path |
| `dump dir` | Set dump directory |
| `dumpdir dir` | Set dump directory |
| `get` | Load file from disk |
| `put` | Save to disk |
| `del` | Delete window (must be clean or will fail) |
| `delete` | Delete window (must be clean or will fail) |
| `dot=addr` | Set selection to address |
| `addr=addr` | Set addr file to address |
| `limit=addr` | Restrict addressing to range |

**Read Format**:
```
winid tagwidth
```
- `winid`: Window ID (decimal)
- `tagwidth`: Width of tag in characters

**Usage**:
```bash
# Send commands
echo "command" | 9p write acme/$winid/ctl

# Read window metadata
9p read acme/$winid/ctl
```

**Examples**:
```bash
# Set window name
echo "name /path/to/file.txt" | 9p write acme/$winid/ctl

# Mark clean before deleting
echo "clean" | 9p write acme/$winid/ctl
echo "del" | 9p write acme/$winid/ctl

# Make window visible
echo "show" | 9p write acme/$winid/ctl
```

**Notes**:
- Commands are executed synchronously
- `del` fails if window is dirty
- `name` sets the string shown in tag and used by Get/Put
- Multiple commands can be sent with multiple writes

### addr

**Purpose**: Set and query text addresses.

**Operations**:
- **Write**: Set current address
- **Read**: Get resolved byte positions

**Address Syntax**:

| Pattern | Meaning |
|---------|---------|
| `0` | Start of file |
| `$` | End of file |
| `.` | Current selection |
| `42` | Absolute position (rune #42) |
| `#42` | Absolute byte offset |
| `/regexp/` | Forward search |
| `-/regexp/` | Backward search |
| `+n` | n runes forward |
| `-n` | n runes backward |
| `0,$` | Entire file |
| `.,+#100` | Current position to 100 bytes forward |
| `/start/,/end/` | From start to end pattern |

**Read Format**:
```
q0 q1
```
- `q0`: Start byte offset (decimal)
- `q1`: End byte offset (decimal)

**Usage**:
```bash
# 1. Write address expression
echo "address" | 9p write acme/$winid/addr

# 2. Read to get byte positions
9p read acme/$winid/addr

# 3. Use data/xdata to read/write at that position
```

**Examples**:
```bash
# Select entire file
echo "0,\$" | 9p write acme/$winid/addr
9p read acme/$winid/addr  # Returns "0 12345" (example)

# Find pattern
echo "/TODO/" | 9p write acme/$winid/addr
9p read acme/$winid/addr  # Returns byte positions of match

# Set to current selection
echo "." | 9p write acme/$winid/addr
9p read acme/$winid/addr
```

**Notes**:
- Addresses are in runes for most syntax, byte offsets for `#`
- Reading addr returns byte offsets, not rune positions
- Failed address (no match, out of range) returns error
- Current address persists for data operations

### body

**Purpose**: Direct access to window body text.

**Operations**:
- **Read**: Read entire body content
- **Write**: Replace entire body content

**Format**: Raw UTF-8 text

**Usage**:
```bash
# Read all body text
9p read acme/$winid/body

# Replace all body text
echo "content" | 9p write acme/$winid/body
```

**Examples**:
```bash
# Read entire body
text=$(9p read acme/$winid/body)

# Replace entire body
echo "New content\n" | 9p write acme/$winid/body

# Write multiline content
cat <<'EOF' | 9p write acme/$winid/body
Line 1
Line 2
Line 3
EOF
```

**Notes**:
- Reading returns all text regardless of size
- Writing replaces all existing text
- For selective editing, use addr + data instead
- Marks window dirty after write

### tag

**Purpose**: Direct access to window tag text.

**Operations**:
- **Read**: Read entire tag content
- **Write**: Replace entire tag content

**Format**: Raw UTF-8 text (typically single line)

**Usage**:
```bash
# Read current tag
9p read acme/$winid/tag

# Replace tag text
echo "new tag" | 9p write acme/$winid/tag
```

**Examples**:
```bash
# Add command to tag
current_tag=$(9p read acme/$winid/tag)
echo "$current_tag MyCommand" | 9p write acme/$winid/tag

# Set tag with filename and commands
echo "/path/file.txt Del Get Put | fmt" | 9p write acme/$winid/tag
```

**Notes**:
- Tag is conventionally one line
- Standard commands: Del, Get, Put, Undo, Redo
- Users expect to find commands in tag
- Pipe commands on right side of bar `|`

### data

**Purpose**: Read/write text at current address, advancing position.

**Operations**:
- **Read**: Read text from current address, advance addr
- **Write**: Write text at current address, advance addr

**Format**: Raw UTF-8 text

**Usage**:
```bash
# 1. Set address with addr file
echo "address" | 9p write acme/$winid/addr

# 2. Read or write data
9p read acme/$winid/data     # Read
echo "text" | 9p write acme/$winid/data  # Write

# 3. Address automatically advances
```

**Examples**:
```bash
# Read first 100 bytes
echo "#0,#100" | 9p write acme/$winid/addr
9p read acme/$winid/data  # Addr now at #100

# Replace word at selection
echo "." | 9p write acme/$winid/addr
echo "replacement" | 9p write acme/$winid/data
```

**Notes**:
- Address advances after each operation
- For multiple operations at same position, use xdata
- Reading less than full range advances partially
- Returns EOF when reading past end of file

### xdata

**Purpose**: Read text at current address without advancing position.

**Operations**:
- **Read**: Read text from current address, don't advance

**Format**: Raw UTF-8 text

**Usage**:
```bash
# 1. Set address with addr file
echo "address" | 9p write acme/$winid/addr

# 2. Read xdata (multiple times possible)
9p read acme/$winid/xdata

# 3. Address remains unchanged
```

**Examples**:
```bash
# Read same text multiple times
echo "/pattern/" | 9p write acme/$winid/addr
text1=$(9p read acme/$winid/xdata)  # Addr unchanged
text2=$(9p read acme/$winid/xdata)  # Same text again
```

**Notes**:
- Only readable, not writable
- Useful for inspecting text without side effects
- Commonly used when checking context around a position

### event

**Purpose**: Receive and acknowledge user interaction events.

**Operations**:
- **Read**: Get next event from queue
- **Write**: Acknowledge event

**Format**: Binary structure (see event-protocol.md for details)
```
c1 c2 q0 q1 flag nr text
```
- 5 fields of 4 bytes each (c1, c2, q0, q1, flag)
- 1 field of 1 byte (nr character count)
- Variable-length UTF-8 text

**Event Types** (c2):

| Type | Name | Description |
|------|------|-------------|
| `L` | Look | Button 3 click (reference lookup) |
| `l` | look | Return from look |
| `D` | Delete | Text deleted |
| `d` | delete | Acknowledged delete |
| `I` | Insert | Text inserted |
| `i` | insert | Acknowledged insert |
| `X` | Execute | Button 2 click in tag |
| `x` | execute | Return from execute |

**Event Origins** (c1):

| Origin | Description |
|--------|-------------|
| `E` | Keyboard input in body |
| `F` | File input (Get, Undo, etc.) |
| `K` | Keyboard input in tag |
| `M` | Mouse action |

**Usage**:
Event handling requires binary I/O and cannot be done with the `9p` command alone.
Use a programming language (Python, Go, C) with either:
- Subprocess calls for window management + direct file I/O for events
- A 9P library (py9p, 9fans.net/go/plan9/client)

**Process**:
```
1. Open event file for binary read/write
2. Loop: read event (binary format)
3. Parse event structure
4. Handle based on type
5. Write event back to acknowledge
6. Unacknowledged events block acme
```

**Notes**:
- See event-protocol.md for complete parsing details
- Binary format: 5×4-byte integers + 1 byte + variable UTF-8
- Must acknowledge events to prevent blocking
- EOF indicates window closed
- Positions (q0, q1) are byte offsets, not runes
- **Cannot use `9p read/write` for events** - binary format required

### errors

**Purpose**: Display error messages in system error window.

**Operations**:
- **Write**: Send error message

**Format**: Raw UTF-8 text

**Usage**:
```bash
# Write error messages
echo "message" | 9p write acme/$winid/errors
```

**Examples**:
```bash
# Write error message
echo "Failed to process file" | 9p write acme/$winid/errors

# Write warning
echo "Warning: invalid input on line 42" | 9p write acme/$winid/errors
```

**Notes**:
- Automatically creates/shows +Errors window
- Messages should end with newline
- Used for tool diagnostics and warnings
- Different from ctl command errors

### rdsel (Plan 9 only)

**Purpose**: Read current selection without using addr.

**Operations**:
- **Read**: Get currently selected text

**Format**: Raw UTF-8 text

**Usage**:
```
text = read(rdsel)
```

**Notes**:
- Plan 9 acme only, not in plan9port
- Convenience for getting selection
- Equivalent to: write(addr, "."), read(xdata)

## File Operation Patterns

### Safe Window Deletion

```bash
# Try to mark clean, then delete
echo "clean" | 9p write acme/$winid/ctl
if echo "del" | 9p write acme/$winid/ctl 2>/dev/null; then
    echo "Window deleted"
else
    echo "Window has unsaved changes"
fi
```

### Atomic Text Replacement

```bash
# Replace entire address range atomically
echo "0,\$" | 9p write acme/$winid/addr
echo "$new_content" | 9p write acme/$winid/data
```

### Inspect Without Modifying

```bash
# Check what's at an address
echo "/pattern/" | 9p write acme/$winid/addr
positions=$(9p read acme/$winid/addr)
text=$(9p read acme/$winid/xdata)  # Doesn't advance addr
```

### Append Text

```bash
# Append to end of file
echo "\$" | 9p write acme/$winid/addr
echo "appended text" | 9p write acme/$winid/data
```

### Create Named Window

```bash
# Create and set up new window
winid=$(9p read acme/new/ctl)
echo "name /path/to/file" | 9p write acme/$winid/ctl
echo "/path/to/file Del Get Put" | 9p write acme/$winid/tag
echo "Initial content" | 9p write acme/$winid/body
echo "show" | 9p write acme/$winid/ctl
```
