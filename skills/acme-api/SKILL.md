---
name: acme-api
description: Programming the acme text editor 9P API. Use when writing code to control acme windows, handle events, manipulate text, or integrate with acme via its file interface.
version: 1.1.0
---

# Acme 9P API Programming

This skill provides comprehensive guidance for programming against the acme text editor's 9P file interface, enabling you to create tools and scripts that interact with acme windows in any language with a 9P client.

## Overview

Acme exposes its functionality through a 9P file server. On plan9port (Plan 9 from User Space), you access this via the `9p` command-line tool. On native Plan 9, acme is typically mounted at `/mnt/acme`.

**This skill focuses on plan9port**, which is the most common deployment. Where behavior differs, native Plan 9 specifics are noted.

### Accessing Acme's Namespace

**plan9port** (recommended):
- Use `9p ls acme` to list files
- Use `9p read acme/path` to read
- Use `9p write acme/path` or `echo data | 9p write acme/path` to write

**Native Plan 9**:
- Direct filesystem access at `/mnt/acme`
- Use standard file operations

### Window File Hierarchy

```
acme/
├── index          # List of all windows
├── new/           # Create new windows
│   └── ctl        # Read to create window and get ID
└── $winid/        # Per-window directory (e.g., acme/5/)
    ├── addr       # Set/query text addresses
    ├── body       # Window body content
    ├── ctl        # Control commands
    ├── data       # Read/write at current address
    ├── errors     # Error messages
    ├── event      # Event stream
    ├── tag        # Window tag content
    └── xdata      # Read without address advance
```

## Quick Start

### Basic Window Control Pattern

1. **Create or attach to window**: Read window ID from `new/ctl` or parse `index`
2. **Set up event handling**: Open `event` for reading
3. **Control the window**: Write commands to `ctl`
4. **Manipulate text**: Use `addr` to set position, `data` to read/write
5. **Process events**: Read from `event` in a loop

### Common Operations

**Create a new window:**
```bash
winid=$(9p read acme/new/ctl | awk '{print $1}')
# Use $winid for all subsequent operations
```

**Execute a command:**
```bash
echo "clean" | 9p write acme/$winid/ctl
echo "show" | 9p write acme/$winid/ctl
echo "name /path/to/file" | 9p write acme/$winid/ctl
```

**Set text address:**
```bash
echo "0" | 9p write acme/$winid/addr        # Start of file
echo "\$" | 9p write acme/$winid/addr       # End of file
echo "0,\$" | 9p write acme/$winid/addr     # Entire file
9p read acme/$winid/addr                     # Get resolved positions (q0 q1)
```

**Read/write text:**
```bash
# Write to body (replaces all)
echo "New content" | 9p write acme/$winid/body

# Read entire body
9p read acme/$winid/body

# Append to end using addr + data
echo "\$" | 9p write acme/$winid/addr
echo "appended text" | 9p write acme/$winid/data
```

**Handle events:**
```
Read acme/$winid/event in a loop
Each event is 5×rune(4bytes) + type(1byte) + text(variable)
Parse origin, type, position info, and event text
(Event handling requires binary parsing - see event-protocol.md)
```

## File Operations Reference

For detailed information on each file's purpose, format, and operations, see **file-reference.md**.

Key files:
- **ctl**: Send control commands, get window info
- **event**: Receive user interactions (clicks, typing, execution)
- **addr**: Set the current text address for data operations
- **body/tag**: Direct access to window content
- **data/xdata**: Read/write at current address

## Event Protocol

The event stream is the core of interactive acme tools. For complete event format details, parsing strategies, and handling patterns, see **event-protocol.md**.

Event format: `c1 c2 q0 q1 flag nr text`
- `c1`: Origin (E=keyboard, K=tag, M=mouse)
- `c2`: Type (x=execute, l=look, i=insert, d=delete, etc.)
- `q0, q1`: Text positions (byte offsets)
- `flag`: Additional state (chorded operations, etc.)
- `nr`: Number of runes in text
- `text`: The actual text involved

## Programming Language Considerations

**Language-agnostic**: The acme API only requires access to the 9P interface. Any language can interact with acme.

### plan9port Access Methods

1. **Shell scripts** (simplest): Use `9p read` and `9p write` commands
2. **Subprocess approach**: Call `9p` command from Python, Ruby, etc.
3. **9P library**: Use native 9P client library (Go, Python py9p, etc.)
4. **Direct namespace access**: On some systems, acme namespace is mounted as regular filesystem

Common approaches by language:
- **Shell**: `9p read acme/...`, `9p write acme/...` (recommended for scripts)
- **Python**: subprocess to call `9p`, or use `py9p` library
- **Go**: `9fans.net/go/plan9/client` library
- **C**: Call `9p` via `popen()`, or use `libixp`/`lib9p`
- **Any language**: Shell out to `9p` command via subprocess

## Working with Supporting Files

This skill includes detailed supporting documentation:

- **file-reference.md**: Complete reference for all acme files
- **event-protocol.md**: Event format, parsing, and handling patterns
- **examples.md**: Code examples in multiple languages

Read these files when you need detailed information about specific aspects of the API.

## Common Patterns

### Event Loop Template

```
1. Open $winid/event for reading
2. Loop:
   a. Read one event (5×4 bytes + 1 byte + variable text)
   b. Parse origin, type, positions, text
   c. Handle event based on type
   d. For certain events, write response to event file
3. On EOF or error, clean up and exit
```

### Text Manipulation Template

```
1. Write address to $winid/addr (e.g., "0,$" for entire body)
2. Read $winid/addr to get byte positions (q0, q1)
3. Read from or write to $winid/data
4. For multiple operations, use xdata to prevent address advancing
```

### Window Creation Template

```bash
# Create window and get ID (extract first field)
winid=$(9p read acme/new/ctl | awk '{print $1}')

# Set window name
echo "name /path/to/file" | 9p write acme/$winid/ctl

# Set up tag with commands
echo "/path/to/file Del Get Put" | 9p write acme/$winid/tag

# Write initial content
echo "Initial text" | 9p write acme/$winid/body

# Make visible
echo "show" | 9p write acme/$winid/ctl
```

## Key Principles

1. **All positions are byte offsets**, not rune offsets (important for UTF-8)
2. **Events must be acknowledged**: Write the event back to prevent blocking
3. **Addresses are stateful**: The `addr` file maintains current position
4. **Clean/dirty state matters**: Mark windows clean/dirty to affect close behavior
5. **Tag is for commands**: Users expect standard acme commands in tag
6. **Body is for content**: Keep user text in body, tool output can go to either

## Error Handling

- Failed operations on `ctl` return errors via read
- Invalid addresses on `addr` return errors
- Event stream EOF indicates window closed
- Check file operations for errors like any system I/O

## Best Practices

1. **Always handle events**: Unread events block acme
2. **Use addr for complex operations**: Don't manually calculate positions
3. **Preserve user intent**: Mark clean/dirty appropriately
4. **Follow acme conventions**: Put commands in tag, content in body
5. **Clean up on exit**: Close files, delete temporary windows
6. **Test with `window`**: Use `window` command to verify behavior

## References

- Plan 9 acme(4) man page
- Russ Cox's "Acme: A User Interface for Programmers"
- /sys/src/cmd/acme on Plan 9 source
- 9fans.net acme documentation

---

When you need specific details about file operations or event handling, read the supporting files in this skill directory.
