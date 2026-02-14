---
name: rc-shell
description: Plan 9 rc shell scripting syntax and conventions. Use when writing .rc shell scripts or working with Plan 9 rc syntax.
user-invocable: false
---

# Plan 9 rc Shell Scripting

Reference for writing shell scripts using Plan 9 rc syntax (`.rc` files).

## File Setup

**Shebang:**
```
#!/usr/bin/env rc
```
Always use `/usr/bin/env rc` (not a direct path like `/usr/local/plan9/bin/rc`) because plan9port installation locations vary across systems.

**File extension:** `.rc`

## Core Syntax Differences from Bash/sh

### Variables

**Assignment** (NO spaces around `=`):
```
name=value
path=(/bin /usr/bin)
```

**Reference** (NO `$` for simple cases in some contexts, but use `$` for safety):
```
echo $name
for(file in *.txt) echo $file
```

**Lists** (space-separated in parentheses):
```
files=(foo.txt bar.txt baz.txt)
echo $files          # prints all elements
echo $files(1)       # prints first element (1-indexed)
echo $files(2-)      # prints from second element onward
```

### Command Substitution

**Backticks** (like classic sh):
```
files=`{ls *.txt}
date=`{date}
```

### Quoting

**Single quotes** for literals:
```
echo 'hello $world'   # prints: hello $world
```

**Double quotes** are NOT special in rc - use single quotes or concatenation.

### Control Structures

**if statement** (note: condition is a command, `if not` is a separate statement):
```
if(test -f file.txt) {
    echo 'file exists'
}
if not {
    echo 'file does not exist'
}

# Single statement form
if(test -f file.txt)
    echo 'exists'
if not
    echo 'does not exist'
```

**for loop:**
```
for(file in *.txt) {
    echo $file
}

for(i in 1 2 3 4 5) {
    echo iteration $i
}
```

**while loop:**
```
while(test $count -lt 10) {
    echo $count
    count=`{echo $count + 1 | bc}
}
```

**switch statement:**
```
switch($file) {
case *.txt
    echo 'text file'
case *.md
    echo 'markdown file'
case *
    echo 'other file'
}
```

### Functions

**Definition:**
```
fn myfunction {
    echo 'arguments:' $*
    echo 'first arg:' $1
}

# Call it
myfunction foo bar baz
```

**Local variables** (use `local` keyword or just assign):
```
fn myfunction {
    local=(old $local)
    # ... function body
}
```

### Exit Status

**Check exit status:**
```
if(grep -q pattern file.txt) {
    echo 'found'
}
```

**Explicit status check:**
```
command
status=$status
if(~ $status 0) {
    echo 'success'
}
```

### Pattern Matching

**The `~` operator:**
```
if(~ $file *.txt) {
    echo 'text file'
}

if(~ $#list 0) {
    echo 'empty list'
}
```

### Pipelines and Redirection

**Standard pipelines:**
```
cat file.txt | grep pattern | sort
```

**Redirection:**
```
echo hello >file.txt          # write
echo world >>file.txt         # append
command >[2=1]                # redirect stderr to stdout
command >[2]/dev/null         # redirect stderr to /dev/null
```

### Important Operators

- `~` - pattern match
- `$#var` - count elements in list
- `$var(N)` - Nth element of list (1-indexed)
- `$var(N-)` - elements from N onward
- `$^var` - concatenate list elements (no spaces)
- `$$var` - pid of command

### Comments

```
# This is a comment
echo hello  # inline comment
```

## Common Patterns

### Check if variable is set
```
if(~ $#var 0) {
    echo 'var is not set'
}
```

### Iterate over arguments
```
fn process {
    for(arg in $*) {
        echo processing $arg
    }
}
```

### Build path list
```
path=($home/bin /usr/local/bin /usr/bin /bin)
```

### Conditional execution
```
test -f file.txt && echo 'exists'
test -f file.txt || echo 'does not exist'
```

### Here documents
```
cat <<EOF
line 1
line 2
EOF
```

## Key Differences from Bash

1. **No `$()` syntax** - use backticks for command substitution
2. **Lists are first-class** - use parentheses for lists
3. **1-indexed arrays** - `$list(1)` is first element
4. **`if not` instead of `else`** - written as a separate statement, not on same line
5. **No `[[ ]]` or `[ ]`** - use `test` command or `~` operator
6. **Pattern matching with `~`** - not `==` or `=`
7. **`$#var` for count** - not `${#var}`
8. **Different redirection** - `>[2=1]` not `2>&1`
9. **No parameter expansion** - no `${var:-default}` syntax
10. **Functions use `fn`** - not `function` keyword

## Best Practices

1. Always use `#!/usr/bin/env rc` shebang
2. Use meaningful variable names
3. Quote variables when word splitting matters
4. Use `~` for pattern matching
5. Remember lists are 1-indexed
6. Write `if not` as a separate statement on a new line
7. Prefer `test` for file/string comparisons
8. Use `$#var` to check if variable is set
9. Leverage list operations for cleaner code
10. Keep scripts simple and readable

## Common Gotchas

- **NO spaces around `=`** in assignments
- **Double quotes are NOT special** - use single quotes
- **Arrays are 1-indexed**, not 0-indexed
- **`if not` is a separate statement** - write it on a new line, not `} if not {`
- **No `${}` parameter expansion**
- **Different redirection syntax** from bash
- **`test` command behavior** may differ from bash `test`

## Resources

For detailed documentation, see `9 man rc` or the Plan 9 documentation.
