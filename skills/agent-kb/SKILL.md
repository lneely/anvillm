---
name: agent-kb
intent: knowledge, learning
description: Query and maintain a local knowledge base of code insights and architecture notes. Use when encountering unfamiliar code, needing context beyond training, or when asked to save/update learned knowledge.
---

# Agent Knowledge Base

**Location:** `~/doc/agent-kb/`

**File format:** Follows **denote** skill conventions (load denote skill for naming/frontmatter rules).

## Usage

**Search before answering:**
```bash
grep -il "keyword" ~/doc/agent-kb/*.md
ls -1 ~/doc/agent-kb/
```

**Read relevant entries before researching code.**

## Creating Entries

1. Use denote naming: `date +%Y%m%dT%H%M%S` → `{timestamp}--{title}__{tags}.md`
2. Add frontmatter (see denote skill): `title`, `date`, `tags`, `identifier`, `verified`, `source`
3. One topic per file
4. Link to other entries: `denote:{identifier}` (e.g., `denote:20260130T171856`)
5. Don't auto-follow cross-references unless directly relevant

## Updating Entries

1. Read existing entry
2. Update content
3. Preserve identifier and filename
4. Update `verified` date in frontmatter
5. Add note on last line: `Updated YYYY-MM-DD: <what changed>`

## Staleness

| Age | Action |
|-----|--------|
| ≤30 days | Trust, use directly |
| 31-90 days | Use with caution, flag as potentially stale |
| 90+ days | Verify against code before using |

When confirmed accurate: update `verified` date.

## What Belongs

**YES:**
- Code patterns, architecture, service behavior
- API contracts, data schemas, integration points
- Non-obvious implementation details
- Verified facts from code/docs

**NO:**
- Speculation or design proposals
- "We could do X" alternatives
- Unverified assumptions
- Temporary workarounds

## Feedback Loop

After code exploration:
- New knowledge → offer to create entry
- KB contradicts code → stop, report discrepancy, ask to update
- KB confirmed → optionally note verification

**If KB contradicts code:** Stop and report. Don't silently ignore KB errors.
