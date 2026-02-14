# Acme Event Protocol

Complete reference for parsing and handling events from the acme event file.

## Event Structure

Events are binary structures read from `/mnt/acme/$winid/event`:

```
Byte Offset | Size | Type | Name  | Description
------------|------|------|-------|---------------------------
0           | 4    | rune | c1    | Origin character
4           | 4    | rune | c2    | Event type character
8           | 4    | int  | q0    | Start byte offset
12          | 4    | int  | q1    | End byte offset
16          | 4    | int  | flag  | Flags (chorded, arg)
20          | 1    | byte | nr    | Number of runes in text (NOT bytes)
21          | var  | utf8 | text  | Event text (nr runes)
```

**Total size**: 21 + (variable UTF-8 bytes for nr runes)

## Event Fields

### c1: Origin Character

Indicates where the event originated:

| c1 | Source | Description |
|----|--------|-------------|
| `E` | Body keyboard | Typed in window body |
| `F` | File operation | From Get, Put, Undo, etc. |
| `K` | Tag keyboard | Typed in window tag |
| `M` | Mouse | Mouse click or drag |

### c2: Event Type Character

The action that occurred:

| c2 | Type | Description | Typical Response |
|----|------|-------------|------------------|
| `L` | Look | Button 3 click (reference) | Lookup, open file, show definition |
| `l` | look | Return from look | Usually ignore |
| `D` | Delete | Text deleted by user | Accept or reject |
| `d` | delete | Delete acknowledged | Usually ignore |
| `I` | Insert | Text inserted by user | Accept or reject |
| `i` | insert | Insert acknowledged | Usually ignore |
| `X` | Execute | Button 2 click in tag | Execute command |
| `x` | execute | Return from execute | Usually ignore |

**Uppercase vs lowercase**:
- **Uppercase (L, D, I, X)**: Action initiated, waiting for response
- **Lowercase (l, d, i, x)**: Action completed/acknowledged

### q0, q1: Text Positions

Byte offsets (NOT rune offsets) into the window text:

- `q0`: Start position (inclusive)
- `q1`: End position (exclusive)
- Range: `[q0, q1)` contains the text involved in the event

**Notes**:
- Zero-based byte offsets
- For point events (clicks), `q0 == q1`
- For range events (selections), `q0 < q1`
- Positions are in body or tag depending on context

### flag: Event Flags

Bitfield with additional information:

| Bit | Mask | Name | Description |
|-----|------|------|-------------|
| 0 | 0x1 | | Unused |
| 1 | 0x2 | Chorded | Event was chorded with previous |
| 2 | 0x4 | Arg | Has argument (selected text) |
| 3 | 0x8 | | Unused |

**Common flag values**:
- `0`: Simple event
- `2`: Chorded operation
- `4`: Has argument (text selected before action)
- `6`: Chorded with argument

### nr: Rune Count

Number of runes (NOT bytes) in the following text field.

**Important**: This is a rune count, but you must decode UTF-8 to get the actual bytes to read.

### text: Event Text

UTF-8 encoded text containing:
- For Insert/Delete: The inserted/deleted text
- For Execute: The command executed
- For Look: The looked-up text

**Length**: Decode `nr` runes worth of UTF-8

## Parsing Events

### Binary Parsing Pattern

```
# Pseudocode for reading one event
c1 = read_rune(4 bytes)
c2 = read_rune(4 bytes)
q0 = read_int32(4 bytes)
q1 = read_int32(4 bytes)
flag = read_int32(4 bytes)
nr = read_byte(1 byte)
text = read_utf8_runes(nr runes)
```

### Handling Rune Count

The `nr` field is the number of **runes**, not bytes. To read the text:

1. Read `nr` as a single byte
2. Read and decode UTF-8 runes until you've got `nr` runes
3. Variable byte length depending on UTF-8 encoding

**Example**: `nr=5` might be:
- 5 bytes: "hello"
- 10 bytes: "h\xC3\xA9llo\xE2\x80\xA6" (héllo…)
- 15 bytes: 5 runes with CJK characters

### Language-Specific Parsing

**C/C++**:
```c
struct Event {
    int c1;     // rune as int
    int c2;     // rune as int
    int q0;
    int q1;
    int flag;
    unsigned char nr;
    char text[256];  // Variable length, up to nr runes
};

// Read event from fd
read(fd, &e.c1, 4);
read(fd, &e.c2, 4);
read(fd, &e.q0, 4);
read(fd, &e.q1, 4);
read(fd, &e.flag, 4);
read(fd, &e.nr, 1);
// Read nr runes worth of UTF-8
read_utf8_runes(fd, e.text, e.nr);
```

**Go**:
```go
import "encoding/binary"

type Event struct {
    C1, C2     rune
    Q0, Q1     int
    Flag       int
    Text       string
}

func readEvent(r io.Reader) (*Event, error) {
    var e Event
    binary.Read(r, binary.LittleEndian, &e.C1)
    binary.Read(r, binary.LittleEndian, &e.C2)
    binary.Read(r, binary.LittleEndian, &e.Q0)
    binary.Read(r, binary.LittleEndian, &e.Q1)
    binary.Read(r, binary.LittleEndian, &e.Flag)

    var nr byte
    binary.Read(r, binary.LittleEndian, &nr)

    // Read nr runes
    text := make([]byte, 0)
    runeCount := 0
    for runeCount < int(nr) {
        r, _, _ := bufio.NewReader(r).ReadRune()
        text = append(text, []byte(string(r))...)
        runeCount++
    }
    e.Text = string(text)
    return &e, nil
}
```

**Python** (using struct):
```python
import struct
import os

def read_event(fd):
    # Read fixed-size fields (little-endian integers)
    data = os.read(fd, 21)
    c1, c2, q0, q1, flag, nr = struct.unpack('<IIIIIb', data)

    # Read nr runes worth of UTF-8
    text_bytes = b''
    rune_count = 0
    while rune_count < nr:
        byte = os.read(fd, 1)
        text_bytes += byte
        # Count complete UTF-8 characters
        try:
            text_bytes.decode('utf-8')
            rune_count += 1
        except UnicodeDecodeError:
            continue

    return {
        'c1': chr(c1),
        'c2': chr(c2),
        'q0': q0,
        'q1': q1,
        'flag': flag,
        'text': text_bytes.decode('utf-8')
    }
```

## Event Handling Patterns

### Basic Event Loop

```
1. Open /mnt/acme/$winid/event
2. Loop:
   a. Read event
   b. Parse event structure
   c. Handle based on (c1, c2) combination
   d. Write event back to acknowledge
   e. Break on EOF (window closed)
3. Close file
```

### Event Acknowledgement

**Critical**: Events MUST be acknowledged by writing them back:

```
event = read_event()
# Process event
write_event(event)  # Write back to acknowledge
```

**What to write back**:
- For accepted events: Write same event
- For rejected events: Write event with different type

**Consequences of not acknowledging**:
- Event queue blocks
- Acme becomes unresponsive for that window
- Other events pile up

### Standard Event Handlers

#### Execute Events (X)

When user Button-2 clicks a command:

```
if c1 == 'M' and c2 == 'X':
    # Mouse execute in tag
    command = event.text

    if command == "MyCommand":
        # Handle custom command
        do_something()
        # Write back to acknowledge
        write_event(event)
    else:
        # Unknown command, let acme handle it
        # Change X to x to reject
        event.c2 = 'x'
        write_event(event)
```

**Execute event flow**:
1. User Button-2 clicks text
2. You receive `X` event with command text
3. If you handle it: acknowledge with same event
4. If you don't handle it: change to `x` and write back
5. Acme shows or executes command

#### Look Events (L)

When user Button-3 clicks (right-click):

```
if c2 == 'L':
    # User is looking up text
    lookup_text = event.text

    # Open file, search documentation, etc.
    handle_lookup(lookup_text)

    # Acknowledge
    write_event(event)
```

**Look event flow**:
1. User Button-3 clicks word or selection
2. You receive `L` event with text
3. Perform lookup (open file, search, definition)
4. Acknowledge event

#### Insert/Delete Events (I/D)

When user types or deletes:

```
if c2 == 'I':
    # Text inserted
    inserted = event.text
    position = event.q0

    # Usually accept all inserts
    write_event(event)

elif c2 == 'D':
    # Text deleted
    deleted = event.text
    start = event.q0
    end = event.q1

    # Usually accept all deletes
    write_event(event)
```

**Use cases**:
- Syntax checking as user types
- Auto-completion
- Formatting enforcement
- Reject certain edits (rare)

### Chorded Operations

When `flag & 0x2` is set, the event is chorded:

```
if flag & 0x2:
    # This is a chorded operation
    # Previous event and this event are related
    # Common: Button-1 select, Button-2 execute on selection
```

**Example**: Cut operation
1. Button-1 drag to select text (flag=0)
2. Button-2 click "Cut" (flag=2, chorded)

### Events with Arguments

When `flag & 0x4` is set, there's an argument:

```
if flag & 0x4:
    # Text was selected before action
    # Argument is available somehow (implementation-specific)
```

## Event Response Patterns

### Accept Event

Write the same event back unchanged:

```
event = read_event()
# Process it
write_event(event)  # Accepted
```

### Reject Event

Change uppercase type to lowercase:

```
event = read_event()
if event.c2 == 'X' and not_my_command(event.text):
    event.c2 = 'x'  # Reject, let acme try
    write_event(event)
```

### Event Type Conversions

| Received | Write Back | Meaning |
|----------|------------|---------|
| `X` | `X` | Accepted, command handled |
| `X` | `x` | Rejected, let acme try |
| `L` | `L` | Accepted, lookup handled |
| `L` | `l` | Rejected, acme shows text |
| `I` | `I` | Accepted insert |
| `D` | `D` | Accepted delete |

## Complete Event Handling Example

```
# Pseudocode event handler

fd = open(f"/mnt/acme/{winid}/event")

while true:
    event = read_event(fd)
    if event is None:
        break  # EOF, window closed

    # Handle based on origin and type
    if event.c1 == 'M' and event.c2 == 'X':
        # Mouse execute in tag
        if event.text == "Build":
            run_build()
            write_event(fd, event)  # Acknowledge
        elif event.text == "Test":
            run_tests()
            write_event(fd, event)
        else:
            # Unknown command
            event.c2 = 'x'
            write_event(fd, event)

    elif event.c2 == 'L':
        # Look up reference
        open_reference(event.text, event.q0, event.q1)
        write_event(fd, event)

    elif event.c2 in ['I', 'D', 'i', 'd']:
        # Text editing, usually just accept
        write_event(fd, event)

    else:
        # Default: acknowledge unknown events
        write_event(fd, event)

close(fd)
```

## Common Mistakes

### 1. Forgetting to Acknowledge

**Wrong**:
```
while true:
    event = read_event()
    process(event)
    # Forgot to write back!
```

**Right**:
```
while true:
    event = read_event()
    process(event)
    write_event(event)  # Must acknowledge
```

### 2. Byte vs Rune Confusion

**Wrong**:
```
text = read(fd, nr)  # Reads nr bytes, might be wrong!
```

**Right**:
```
text = read_utf8_runes(fd, nr)  # Reads nr runes
```

### 3. Not Checking EOF

**Wrong**:
```
while true:
    event = read_event()
    # Crashes when window closes
```

**Right**:
```
while true:
    event = read_event()
    if event is None:
        break  # Window closed
```

### 4. Wrong Byte Order

Events use **little-endian** integers. Make sure your binary parsing uses correct byte order.

### 5. Not Rejecting Unknown Commands

**Wrong**:
```
if event.c2 == 'X':
    write_event(event)  # Accepts everything!
```

**Right**:
```
if event.c2 == 'X':
    if is_my_command(event.text):
        handle_it()
        write_event(event)
    else:
        event.c2 = 'x'  # Reject
        write_event(event)
```

## Testing Event Handlers

### Manual Testing

1. Run your event handler
2. Click in acme window (generates events)
3. Type text (generates I events)
4. Button-2 click commands (generates X events)
5. Button-3 click text (generates L events)
6. Check handler responds correctly

### Debug Logging

Log all events to see what's happening:

```
event = read_event()
log(f"Event: c1={event.c1} c2={event.c2} q0={event.q0} q1={event.q1} text={event.text}")
write_event(event)
```

### Common Test Cases

1. **Execute custom command**: Button-2 on your command
2. **Execute unknown command**: Button-2 on "Get" - should reject
3. **Look up text**: Button-3 on word
4. **Type text**: Insert characters
5. **Delete text**: Backspace, cut
6. **Close window**: Check EOF handling

## Event Examples by Type

### Execute in Tag (MX)

```
c1='M', c2='X', q0=15, q1=20, flag=0, text="Build"
```
User Button-2 clicked "Build" in tag at positions 15-20.

### Execute with Selection (MX with arg)

```
c1='M', c2='X', q0=100, q1=105, flag=4, text="Look"
```
User selected text, then Button-2 clicked "Look" (has argument).

### Look (ML)

```
c1='M', c2='L', q0=50, q1=54, flag=0, text="main"
```
User Button-3 clicked "main" at positions 50-54.

### Insert (EI)

```
c1='E', c2='I', q0=200, q1=200, flag=0, text="x"
```
User typed 'x' at position 200.

### Delete (ED)

```
c1='E', c2='D', q0=150, q1=155, flag=0, text="hello"
```
User deleted "hello" from positions 150-155.

## Reference

- Plan 9 acme(4) man page, Event section
- /sys/src/cmd/acme source code
- 9fans.net/plan9port/man/man4/acme.html
