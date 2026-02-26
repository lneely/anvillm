---
name: rc-shell
intent: scripting, shell
description: Plan 9 rc shell scripting syntax and conventions. Use when writing .rc shell scripts or working with Plan 9 rc syntax.
user-invocable: false
---

# Plan 9 rc Shell Syntax

Shebang: `#!/usr/bin/env rc` Extension: `.rc`

## Variables

```
name=value                   # NO spaces around =
files=(a b c)                # lists in parens
$name $files(1) $files(2-)   # substitute, 1st element, from 2nd onward
$files(2-4)                  # range
$#files                      # count
$"files                      # join with spaces
$^files                      # concat (no spaces)
```

## Command Substitution

```
result=`{command}            # backticks, NOT $()
result=`split{command}       # split using 'split' instead of $ifs
```

## Control Flow

```
if(test -f file) {
    echo exists
}
if not {                     # separate statement, new line
    echo missing
}

for(i in 1 2 3) { echo $i }
for(i) { echo $i }           # iterates over $* if no list

while(test $n -lt 10) { n=`{echo $n + 1 | bc} }

switch($file) {
case *.txt
    echo text
case *
    echo other
}
```

## Functions & Pattern Matching

```
fn name { echo $* $1 }       # define (all args, first arg)
fn name                      # remove definition
~ $file *.txt                # pattern match (sets $status)
if(~ $#list 0) { echo empty }
```

## Redirection & Pipes

```
>out >>out <in <<EOF         # stdout, append, stdin, here doc
>[2]err >[2=1] >[2=]         # fd 2 to file, fd 2 to fd 1, close fd 2
<[0=3]                       # fd 0 from fd 3
<{cmd} >{cmd} <>{cmd}        # process substitution
cmd1 | cmd2                  # pipe
|[2] |[1=2]                  # pipe fd 2, pipe fd 2 to fd 1
```

## Operators

`; & && || ! @` - sequence, async, and, or, invert, subshell
`^` - concatenate (auto-inserted without whitespace)

## Special Variables

`$*` `$status` `$apid` `$pid` `$ifs` `$path` `$home`

## Built-ins

`. file` `cd` `eval` `exec` `exit` `shift` `wait` `whatis`

## Key Differences from Bash

- Backticks `{cmd}`, not `$(cmd)`
- Lists `(a b c)`, 1-indexed
- `if not` on new line
- `test` or `~`, not `[[ ]]`
- `$#var` not `${#var}`, `$"var` not `"$*"`
- `>[2=1]` not `2>&1`
- No `${var:-default}`
- `fn name` not `function name`
- Single quotes only (double quotes not special)
- Free carets: `$x.c` → `$x^.c`
