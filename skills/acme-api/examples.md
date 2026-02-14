# Acme API Examples

Code examples for common acme programming patterns in multiple languages.

**Note on plan9port**: These examples show different approaches:
- **Shell**: Uses `9p` command (plan9port standard)
- **Python/Go/C**: Can use `9p` command via subprocess, or access namespace directly if available

For event handling (binary I/O), you'll need Python/Go/C, not just shell scripts.

## Table of Contents

1. [Creating Windows](#creating-windows)
2. [Reading and Writing Text](#reading-and-writing-text)
3. [Event Handling](#event-handling)
4. [Complete Programs](#complete-programs)

## Creating Windows

### Go

**Note**: This example assumes acme namespace is mounted at `/mnt/acme`. For plan9port, use:
- `exec.Command("9p", "read", "acme/new/ctl")` for subprocess approach
- Or `9fans.net/go/plan9/client` library for native 9P

**Direct namespace access** (if available):
```go
package main

import (
    "fmt"
    "io/ioutil"
    "os"
)

func createWindow() (int, error) {
    // Open new/ctl to create window
    f, err := os.Open("/mnt/acme/new/ctl")
    if err != nil {
        return 0, err
    }
    defer f.Close()

    // Read window metadata and extract ID (first field)
    data, err := ioutil.ReadAll(f)
    if err != nil {
        return 0, err
    }

    var winid int
    fmt.Sscanf(string(data), "%d", &winid)
    return winid, nil
}

func setWindowName(winid int, name string) error {
    path := fmt.Sprintf("/mnt/acme/%d/ctl", winid)
    f, err := os.OpenFile(path, os.O_WRONLY, 0)
    if err != nil {
        return err
    }
    defer f.Close()

    _, err = f.WriteString(fmt.Sprintf("name %s", name))
    return err
}

func main() {
    winid, err := createWindow()
    if err != nil {
        panic(err)
    }
    fmt.Printf("Created window %d\n", winid)

    err = setWindowName(winid, "/tmp/test.txt")
    if err != nil {
        panic(err)
    }
}
```

### Python

**Note**: This example assumes acme namespace is mounted. For plan9port, use subprocess:
```python
import subprocess
winid = subprocess.run(['9p', 'read', 'acme/new/ctl'],
                       capture_output=True, text=True).stdout.strip()
```

**Direct namespace access** (if available):
```python
import os

def create_window():
    """Create a new acme window and return its ID."""
    with open('/mnt/acme/new/ctl', 'r') as f:
        # Extract first field (window ID) from ctl output
        winid = int(f.read().split()[0])
    return winid

def set_window_name(winid, name):
    """Set the name of a window."""
    path = f'/mnt/acme/{winid}/ctl'
    with open(path, 'w') as f:
        f.write(f'name {name}')

def write_body(winid, text):
    """Write text to window body."""
    path = f'/mnt/acme/{winid}/body'
    with open(path, 'w') as f:
        f.write(text)

# Usage
winid = create_window()
print(f"Created window {winid}")
set_window_name(winid, '/tmp/example.txt')
write_body(winid, 'Hello from Python!\n')
```

### C

**Note**: This example assumes acme namespace mounted. For plan9port, use `popen()`:
```c
FILE *fp = popen("9p read acme/new/ctl", "r");
fscanf(fp, "%d", &winid);
pclose(fp);
```

**Direct namespace access** (if available):
```c
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <fcntl.h>
#include <unistd.h>

int create_window(void) {
    int fd, winid;
    char buf[32];

    fd = open("/mnt/acme/new/ctl", O_RDONLY);
    if (fd < 0) {
        perror("open new/ctl");
        return -1;
    }

    read(fd, buf, sizeof(buf));
    close(fd);

    /* Extract window ID (first integer field) */
    winid = atoi(buf);
    return winid;
}

int set_window_name(int winid, const char *name) {
    int fd;
    char path[256];
    char cmd[512];

    snprintf(path, sizeof(path), "/mnt/acme/%d/ctl", winid);
    fd = open(path, O_WRONLY);
    if (fd < 0) {
        perror("open ctl");
        return -1;
    }

    snprintf(cmd, sizeof(cmd), "name %s", name);
    write(fd, cmd, strlen(cmd));
    close(fd);
    return 0;
}

int main(void) {
    int winid;

    winid = create_window();
    if (winid < 0) {
        return 1;
    }

    printf("Created window %d\n", winid);
    set_window_name(winid, "/tmp/test.c");

    return 0;
}
```

### Shell (using 9p command)

```bash
#!/bin/sh

# Create new window (extract window ID from first field)
winid=$(9p read acme/new/ctl | awk '{print $1}')
echo "Created window $winid"

# Set window name
echo "name /tmp/example.txt" | 9p write acme/$winid/ctl

# Write to body
echo "Hello from shell!" | 9p write acme/$winid/body

# Add command to tag
echo " | fmt" | 9p write acme/$winid/tag
```

## Reading and Writing Text

### Go: Text Manipulation

```go
package main

import (
    "fmt"
    "io/ioutil"
    "os"
)

// Replace all text in window
func replaceAll(winid int, text string) error {
    path := fmt.Sprintf("/mnt/acme/%d/body", winid)
    return ioutil.WriteFile(path, []byte(text), 0)
}

// Read all text from window
func readAll(winid int) (string, error) {
    path := fmt.Sprintf("/mnt/acme/%d/body", winid)
    data, err := ioutil.ReadFile(path)
    return string(data), err
}

// Replace text at specific address
func replaceAt(winid int, addr, text string) error {
    addrPath := fmt.Sprintf("/mnt/acme/%d/addr", winid)
    dataPath := fmt.Sprintf("/mnt/acme/%d/data", winid)

    // Set address
    if err := ioutil.WriteFile(addrPath, []byte(addr), 0); err != nil {
        return err
    }

    // Write new text
    return ioutil.WriteFile(dataPath, []byte(text), 0)
}

// Get current selection
func getSelection(winid int) (string, error) {
    addrPath := fmt.Sprintf("/mnt/acme/%d/addr", winid)
    xdataPath := fmt.Sprintf("/mnt/acme/%d/xdata", winid)

    // Set address to current selection
    if err := ioutil.WriteFile(addrPath, []byte("."), 0); err != nil {
        return "", err
    }

    // Read selected text
    data, err := ioutil.ReadFile(xdataPath)
    return string(data), err
}
```

### Python: Text Operations

```python
def replace_all(winid, text):
    """Replace all text in window body."""
    with open(f'/mnt/acme/{winid}/body', 'w') as f:
        f.write(text)

def read_all(winid):
    """Read all text from window body."""
    with open(f'/mnt/acme/{winid}/body', 'r') as f:
        return f.read()

def replace_at(winid, addr, text):
    """Replace text at specific address."""
    # Set address
    with open(f'/mnt/acme/{winid}/addr', 'w') as f:
        f.write(addr)

    # Write new text
    with open(f'/mnt/acme/{winid}/data', 'w') as f:
        f.write(text)

def get_selection(winid):
    """Get currently selected text."""
    # Set address to current selection
    with open(f'/mnt/acme/{winid}/addr', 'w') as f:
        f.write('.')

    # Read without advancing
    with open(f'/mnt/acme/{winid}/xdata', 'r') as f:
        return f.read()

def append_text(winid, text):
    """Append text to end of window."""
    replace_at(winid, '$', text)

def find_and_replace(winid, pattern, replacement):
    """Find pattern and replace with new text."""
    addr = f'/{pattern}/'
    replace_at(winid, addr, replacement)
```

### Shell: Text Manipulation

```bash
#!/bin/sh

winid=$1

# Read all text
text=$(9p read acme/$winid/body)

# Replace all text
echo "New content" | 9p write acme/$winid/body

# Replace at specific address
echo "0,5" | 9p write acme/$winid/addr
echo "Hello" | 9p write acme/$winid/data

# Get selection
echo "." | 9p write acme/$winid/addr
9p read acme/$winid/xdata

# Append to end
echo "$" | 9p write acme/$winid/addr
echo "appended text" | 9p write acme/$winid/data
```

## Event Handling

### Go: Complete Event Loop

```go
package main

import (
    "encoding/binary"
    "fmt"
    "io"
    "os"
)

type Event struct {
    C1, C2 rune
    Q0, Q1 int
    Flag   int
    Text   string
}

func readEvent(r io.Reader) (*Event, error) {
    var e Event

    // Read fixed fields
    var buf [21]byte
    if _, err := io.ReadFull(r, buf[:]); err != nil {
        return nil, err
    }

    e.C1 = rune(binary.LittleEndian.Uint32(buf[0:4]))
    e.C2 = rune(binary.LittleEndian.Uint32(buf[4:8]))
    e.Q0 = int(binary.LittleEndian.Uint32(buf[8:12]))
    e.Q1 = int(binary.LittleEndian.Uint32(buf[12:16]))
    e.Flag = int(binary.LittleEndian.Uint32(buf[16:20]))
    nr := int(buf[20])

    // Read text (nr runes)
    textBuf := make([]byte, 0, nr*4)
    runeCount := 0
    for runeCount < nr {
        var ch [4]byte
        n, err := r.Read(ch[:1])
        if err != nil || n == 0 {
            break
        }

        // Determine UTF-8 byte length
        b := ch[0]
        var size int
        if b < 0x80 {
            size = 1
        } else if b < 0xE0 {
            size = 2
        } else if b < 0xF0 {
            size = 3
        } else {
            size = 4
        }

        // Read remaining bytes
        if size > 1 {
            if _, err := io.ReadFull(r, ch[1:size]); err != nil {
                break
            }
        }

        textBuf = append(textBuf, ch[:size]...)
        runeCount++
    }

    e.Text = string(textBuf)
    return &e, nil
}

func writeEvent(w io.Writer, e *Event) error {
    var buf [21]byte

    binary.LittleEndian.PutUint32(buf[0:4], uint32(e.C1))
    binary.LittleEndian.PutUint32(buf[4:8], uint32(e.C2))
    binary.LittleEndian.PutUint32(buf[8:12], uint32(e.Q0))
    binary.LittleEndian.PutUint32(buf[12:16], uint32(e.Q1))
    binary.LittleEndian.PutUint32(buf[16:20], uint32(e.Flag))
    buf[20] = byte(len([]rune(e.Text)))

    if _, err := w.Write(buf[:]); err != nil {
        return err
    }
    _, err := w.Write([]byte(e.Text))
    return err
}

func eventLoop(winid int) error {
    path := fmt.Sprintf("/mnt/acme/%d/event", winid)
    f, err := os.OpenFile(path, os.O_RDWR, 0)
    if err != nil {
        return err
    }
    defer f.Close()

    for {
        e, err := readEvent(f)
        if err != nil {
            if err == io.EOF {
                break // Window closed
            }
            return err
        }

        // Handle event
        switch e.C2 {
        case 'X', 'x': // Execute
            if e.C2 == 'X' {
                handled := handleExecute(winid, e.Text)
                if !handled {
                    e.C2 = 'x' // Reject
                }
            }
        case 'L', 'l': // Look
            if e.C2 == 'L' {
                handleLook(winid, e.Text)
            }
        }

        // Acknowledge event
        if err := writeEvent(f, e); err != nil {
            return err
        }
    }

    return nil
}

func handleExecute(winid int, cmd string) bool {
    switch cmd {
    case "Build":
        fmt.Println("Building...")
        // Run build
        return true
    case "Test":
        fmt.Println("Testing...")
        // Run tests
        return true
    default:
        return false // Not handled
    }
}

func handleLook(winid int, text string) {
    fmt.Printf("Looking up: %s\n", text)
    // Open file, search docs, etc.
}
```

### Python: Event Handler

```python
import os
import struct

class Event:
    def __init__(self, c1, c2, q0, q1, flag, text):
        self.c1 = chr(c1)
        self.c2 = chr(c2)
        self.q0 = q0
        self.q1 = q1
        self.flag = flag
        self.text = text

def read_event(fd):
    """Read one event from file descriptor."""
    # Read fixed fields (21 bytes)
    data = os.read(fd, 21)
    if len(data) < 21:
        return None

    c1, c2, q0, q1, flag, nr = struct.unpack('<IIIIIb', data)

    # Read text (nr runes)
    text_bytes = b''
    rune_count = 0
    while rune_count < nr:
        byte = os.read(fd, 1)
        if not byte:
            break
        text_bytes += byte

        # Check if we have a complete UTF-8 character
        try:
            text_bytes.decode('utf-8')
            rune_count += 1
        except UnicodeDecodeError:
            # Incomplete character, read more
            continue

    text = text_bytes.decode('utf-8')
    return Event(c1, c2, q0, q1, flag, text)

def write_event(fd, event):
    """Write event back to acknowledge."""
    # Pack fixed fields
    data = struct.pack('<IIIIIb',
                       ord(event.c1),
                       ord(event.c2),
                       event.q0,
                       event.q1,
                       event.flag,
                       len(event.text))

    os.write(fd, data)
    os.write(fd, event.text.encode('utf-8'))

def event_loop(winid):
    """Main event loop for window."""
    path = f'/mnt/acme/{winid}/event'
    fd = os.open(path, os.O_RDWR)

    try:
        while True:
            event = read_event(fd)
            if event is None:
                break  # EOF, window closed

            # Handle event
            if event.c2 == 'X':
                # Execute command
                if handle_execute(winid, event.text):
                    # Handled, acknowledge
                    write_event(fd, event)
                else:
                    # Not handled, reject
                    event.c2 = 'x'
                    write_event(fd, event)

            elif event.c2 == 'L':
                # Look up text
                handle_look(winid, event.text)
                write_event(fd, event)

            else:
                # Default: acknowledge
                write_event(fd, event)

    finally:
        os.close(fd)

def handle_execute(winid, cmd):
    """Handle execute command. Return True if handled."""
    if cmd == "Build":
        print("Building...")
        # Run build
        return True
    elif cmd == "Test":
        print("Testing...")
        # Run tests
        return True
    else:
        return False

def handle_look(winid, text):
    """Handle look event."""
    print(f"Looking up: {text}")
    # Open file, search, etc.
```

## Complete Programs

### Shell: Complete Window Manager (plan9port)

```bash
#!/bin/sh
# create-acme-window.sh - Create and set up an acme window

# Create new window (extract window ID from first field)
winid=$(9p read acme/new/ctl | awk '{print $1}')
echo "Created window $winid"

# Set window name
echo "name /tmp/myfile.txt" | 9p write acme/$winid/ctl

# Set up tag with commands
echo "/tmp/myfile.txt Del Get Put | fmt" | 9p write acme/$winid/tag

# Write initial content
cat <<'EOF' | 9p write acme/$winid/body
Welcome to your new acme window!

This window was created programmatically.
You can edit this text and use the commands in the tag.
EOF

# Mark it clean (no unsaved changes)
echo "clean" | 9p write acme/$winid/ctl

# Make window visible
echo "show" | 9p write acme/$winid/ctl

echo "Window $winid is ready"
```

### Python: Text Operations (plan9port subprocess)

```python
#!/usr/bin/env python3
"""
Acme window manager using subprocess calls to 9p command.
Works with plan9port.
"""

import subprocess
import sys

def run_9p(cmd, path, input_data=None):
    """Run 9p command and return output."""
    full_cmd = ['9p', cmd, f'acme/{path}']
    result = subprocess.run(full_cmd,
                           input=input_data,
                           capture_output=True,
                           text=True)
    if result.returncode != 0:
        raise Exception(f"9p {cmd} failed: {result.stderr}")
    return result.stdout

def create_window():
    """Create new window and return ID."""
    output = run_9p('read', 'new/ctl').strip()
    # Extract first field (window ID) from ctl output
    return int(output.split()[0])

def write_ctl(winid, cmd):
    """Send control command to window."""
    run_9p('write', f'{winid}/ctl', cmd + '\n')

def write_body(winid, text):
    """Write text to window body."""
    run_9p('write', f'{winid}/body', text)

def read_body(winid):
    """Read text from window body."""
    return run_9p('read', f'{winid}/body')

def append_text(winid, text):
    """Append text to end of window."""
    run_9p('write', f'{winid}/addr', '$\n')
    run_9p('write', f'{winid}/data', text)

def main():
    # Create window
    winid = create_window()
    print(f"Created window {winid}")

    # Set up window
    write_ctl(winid, 'name /tmp/python-example.txt')
    write_body(winid, 'Hello from Python!\n\n')
    append_text(winid, 'This text was appended.\n')

    # Read back
    content = read_body(winid)
    print(f"Window content:\n{content}")

    # Clean up
    write_ctl(winid, 'clean')
    write_ctl(winid, 'show')

if __name__ == '__main__':
    main()
```

### Go: Simple Acme Tool

```go
package main

import (
    "fmt"
    "log"
    "os"
)

func main() {
    // Create window
    winid, err := createWindow()
    if err != nil {
        log.Fatal(err)
    }

    // Set up window
    setupWindow(winid)

    // Run event loop
    if err := eventLoop(winid); err != nil {
        log.Fatal(err)
    }
}

func createWindow() (int, error) {
    f, err := os.Open("/mnt/acme/new/ctl")
    if err != nil {
        return 0, err
    }
    defer f.Close()

    // Extract window ID (first field from ctl output)
    var winid int
    _, err = fmt.Fscanf(f, "%d", &winid)
    return winid, err
}

func setupWindow(winid int) error {
    // Set name
    ctlPath := fmt.Sprintf("/mnt/acme/%d/ctl", winid)
    if err := writeFile(ctlPath, "name /tmp/mytool"); err != nil {
        return err
    }

    // Set tag
    tagPath := fmt.Sprintf("/mnt/acme/%d/tag", winid)
    if err := writeFile(tagPath, "/tmp/mytool Del Get Put Build Test"); err != nil {
        return err
    }

    // Set initial body
    bodyPath := fmt.Sprintf("/mnt/acme/%d/body", winid)
    return writeFile(bodyPath, "Ready.\n")
}

func writeFile(path, content string) error {
    f, err := os.OpenFile(path, os.O_WRONLY, 0)
    if err != nil {
        return err
    }
    defer f.Close()
    _, err = f.WriteString(content)
    return err
}
```

### Python: Log Viewer

```python
#!/usr/bin/env python3
"""
Simple log viewer in acme.
Watches a log file and displays updates.
"""

import os
import sys
import time
import threading

def tail_file(path, callback):
    """Tail a file and call callback with new lines."""
    with open(path, 'r') as f:
        # Seek to end
        f.seek(0, 2)

        while True:
            line = f.readline()
            if line:
                callback(line)
            else:
                time.sleep(0.1)

def append_to_window(winid, text):
    """Append text to end of window."""
    # Set address to end
    with open(f'/mnt/acme/{winid}/addr', 'w') as f:
        f.write('$')

    # Write text
    with open(f'/mnt/acme/{winid}/data', 'w') as f:
        f.write(text)

def main():
    if len(sys.argv) < 2:
        print("Usage: logview <logfile>")
        sys.exit(1)

    logfile = sys.argv[1]

    # Create window (extract first field from ctl output)
    with open('/mnt/acme/new/ctl', 'r') as f:
        winid = int(f.read().split()[0])

    # Set up window
    with open(f'/mnt/acme/{winid}/ctl', 'w') as f:
        f.write(f'name {logfile}')

    with open(f'/mnt/acme/{winid}/tag', 'w') as f:
        f.write(f'{logfile} Del')

    # Start tailing file in background
    def on_line(line):
        append_to_window(winid, line)

    thread = threading.Thread(target=tail_file, args=(logfile, on_line))
    thread.daemon = True
    thread.start()

    # Run event loop (minimal, just keep window alive)
    event_fd = os.open(f'/mnt/acme/{winid}/event', os.O_RDWR)
    try:
        while True:
            event = read_event(event_fd)
            if event is None:
                break
            # Just acknowledge all events
            write_event(event_fd, event)
    finally:
        os.close(event_fd)

if __name__ == '__main__':
    main()
```

### Shell: Quick Acme Script

```bash
#!/bin/sh
# quick-note.sh - Create a quick note window

# Create window (extract window ID from first field)
winid=$(9p read acme/new/ctl | awk '{print $1}')

# Set name
echo "name /tmp/notes.txt" | 9p write acme/$winid/ctl

# Set tag with commands
echo "/tmp/notes.txt Del Get Put | fmt" | 9p write acme/$winid/tag

# Add timestamp to body
date | 9p write acme/$winid/body
echo "" | 9p write acme/$winid/body

# Show window
echo "show" | 9p write acme/$winid/ctl

echo "Created note window $winid"
```

## Advanced Patterns

### Window Manager

```python
class AcmeWindow:
    """Wrapper for acme window operations."""

    def __init__(self, winid=None):
        if winid is None:
            winid = self.create()
        self.winid = winid

    @staticmethod
    def create():
        """Create new window, return ID."""
        with open('/mnt/acme/new/ctl', 'r') as f:
            # Extract first field (window ID) from ctl output
            return int(f.read().split()[0])

    def ctl(self, cmd):
        """Send control command."""
        with open(f'/mnt/acme/{self.winid}/ctl', 'w') as f:
            f.write(cmd)

    def name(self, name):
        """Set window name."""
        self.ctl(f'name {name}')

    def read_body(self):
        """Read entire body."""
        with open(f'/mnt/acme/{self.winid}/body', 'r') as f:
            return f.read()

    def write_body(self, text):
        """Replace body content."""
        with open(f'/mnt/acme/{self.winid}/body', 'w') as f:
            f.write(text)

    def append(self, text):
        """Append to body."""
        with open(f'/mnt/acme/{self.winid}/addr', 'w') as f:
            f.write('$')
        with open(f'/mnt/acme/{self.winid}/data', 'w') as f:
            f.write(text)

    def delete(self):
        """Delete window."""
        self.ctl('del')

# Usage
win = AcmeWindow()
win.name('/tmp/example.txt')
win.write_body('Hello, acme!\n')
win.append('More text\n')
```

## Testing and Debugging

### Event Logger

```python
#!/usr/bin/env python3
"""Log all events from a window."""

import os
import sys

def main():
    winid = int(sys.argv[1]) if len(sys.argv) > 1 else create_window()

    fd = os.open(f'/mnt/acme/{winid}/event', os.O_RDWR)

    print(f"Logging events for window {winid}")

    try:
        while True:
            event = read_event(fd)
            if event is None:
                break

            print(f"Event: c1={event.c1!r} c2={event.c2!r} "
                  f"q0={event.q0} q1={event.q1} "
                  f"flag={event.flag} text={event.text!r}")

            # Acknowledge
            write_event(fd, event)
    finally:
        os.close(fd)

if __name__ == '__main__':
    main()
```
