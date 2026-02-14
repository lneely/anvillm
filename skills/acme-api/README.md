# Acme 9P API Skill

This skill provides comprehensive guidance for programming the acme text editor's 9P file interface in any programming language.

## What This Skill Does

The acme-api skill helps you write code that interacts with acme windows through the 9P filesystem interface. It provides:

- Complete reference for all acme files (ctl, event, body, addr, etc.)
- Event protocol parsing and handling
- Code examples in multiple languages (Go, Python, C, Shell)
- Quick reference for common operations
- Language-agnostic guidance (works with any 9P client)

## When It's Used

This skill is automatically invoked by Claude when:
- Writing code to control acme windows
- Handling acme events
- Manipulating text in acme programmatically
- Integrating tools with acme

You can also manually invoke it with `/acme-api`

## Files in This Skill

- **SKILL.md** - Main skill file with overview and quick start
- **file-reference.md** - Complete reference for all acme files
- **event-protocol.md** - Event format, parsing, and handling patterns
- **examples.md** - Code examples in multiple languages
- **quick-reference.md** - Fast lookup for common operations

## Structure

The skill is organized to provide:

1. **Quick start** in SKILL.md for common patterns
2. **Detailed references** in supporting files
3. **Working code examples** in multiple languages
4. **Quick lookup** for experienced users

## Topics Covered

### File Operations
- Creating windows (`new/ctl`)
- Control commands (`ctl`)
- Text addressing (`addr`)
- Reading/writing text (`body`, `data`, `xdata`)
- Tag manipulation (`tag`)

### Event Handling
- Event structure (binary format)
- Event types (Execute, Look, Insert, Delete)
- Event parsing in multiple languages
- Acknowledgement patterns
- Common event handling patterns

### Programming Languages
- **Go**: With 9P client or direct file operations
- **Python**: Using struct and os modules
- **C**: Direct system calls
- **Shell**: Using 9p command-line tool
- **Any language**: Via standard file I/O

## Usage Examples

### Creating a Window
```python
with open('/mnt/acme/new/ctl', 'r') as f:
    winid = int(f.read().strip())
```

### Handling Events
```python
event = read_event(fd)
if event.c2 == 'X':  # Execute command
    handle_execute(event.text)
write_event(fd, event)  # Acknowledge
```

### Text Manipulation
```python
with open(f'/mnt/acme/{winid}/addr', 'w') as f:
    f.write('0,$')  # Select all
with open(f'/mnt/acme/{winid}/data', 'w') as f:
    f.write('new content')  # Replace
```

## References

- Plan 9 acme(4) man page
- Russ Cox's "Acme: A User Interface for Programmers"
- Plan 9 from User Space documentation

## Version

1.0.0 - Initial release with comprehensive API coverage
