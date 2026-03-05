---
name: acme-api
intent: api, acme
description: Programming the acme text editor 9P API. Use when writing code to control acme windows, handle events, manipulate text, or integrate with acme via its file interface.
version: 1.1.0
---

# Acme 9P API (plan9port)

Control acme via 9P. Use `9p read/write acme/...`

## Files

```
acme/
├── cons           # command stdout/stderr
├── index          # all windows: id tag_len body_len isdir dirty tag (fields @0,12,24,36,48,60)
├── log            # operations (blocks): id op name (ops: new,zerox,get,put,del)
├── new/ctl        # read returns new window id
└── $id/
    ├── addr       # set/query addresses (write addr, read #m,#n char offsets)
    ├── body       # content (read any offset, write appends)
    ├── ctl        # read: 10 fields (id tag_len body_len isdir dirty width font tabwidth undo redo)
    │              # write: commands (see below)
    ├── data       # read/write at addr, advances to end
    ├── errors     # append to dir/+Errors
    ├── event      # event stream (see protocol)
    ├── tag        # tag content (read any offset, write appends)
    └── xdata      # like data but stops at end, no advance
```

## ctl Commands

`addr=dot` `clean` `dirty` `cleartag` `del` `delete` `dot=addr` `dump cmd` `dumpdir /path` `get` `put` `font /path` `limit=addr` `mark` `nomark` `name /path` `show`

## addr Syntax

`0` `$` `0,$` `123` `123,456` `/regex/` `-/regex/` `.,+#10` `.+/regex/`

## Event Protocol

Format: `origin type q0 q1 flag nr [text]\n`

**Origin:** `E`=file write `F`=other file `K`=keyboard `M`=mouse

**Type:** `D`/`d`=delete body/tag, `I`/`i`=insert body/tag, `L`/`l`=button3 body/tag, `R`/`r`=shift-button3 body/tag, `X`/`x`=button2 body/tag

**flag (bitwise OR):**
- X/x: 1=builtin, 2=expansion follows, 8=chorded (2 msgs follow: arg, origin)
- L/l: 1=acme handles, 2=expansion follows, 4=file/window name

**nr:** char count (0 if text >256, read from data)

**Acknowledge:** flag&1 events write back `origin type q0 q1\n`

## Patterns

**Create:**
```bash
id=$(9p read acme/new/ctl | awk '{print $1}')
echo "name /path" | 9p write acme/$id/ctl
echo "content" | 9p write acme/$id/body
echo "show" | 9p write acme/$id/ctl
```

**Event loop:**
```
read acme/$id/event → parse → handle → ack if flag&1 → repeat (EOF=closed)
```

**Text:**
```bash
echo '0,$' | 9p write acme/$id/addr
9p read acme/$id/data
echo "new" | 9p write acme/$id/data
```

## Key Points

- addr: char offsets (not bytes)
- body/tag: write appends (offset ignored)
- data: uses addr, advances
- xdata: stops at end
- Events: ack flag&1, text >256 elided (nr=0)
- Unread events block acme
