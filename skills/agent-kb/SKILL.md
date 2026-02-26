---
name: agent-kb
intent: knowledge, learning
description: Query and maintain a local knowledge base of code insights and architecture notes. Use when encountering unfamiliar code, needing context beyond training, or when asked to save/update learned knowledge.
---

## Required Skills

This skill requires the **denote** skill for file naming and frontmatter conventions.
Load and follow: `/home/lkn/.claude/skills/denote/SKILL.md`

# Agent Knowledge Base

## Purpose

Leverage accumulated knowledge in `~/doc/agent-kb` to avoid redundant code research. Add new entries when discoveries warrant preservation.

## Answer Priority Hierarchy

When answering user questions, follow this strict priority order:

1. **KB First** - Search `~/doc/agent-kb/` for relevant entries
2. **Code Search** - If KB doesn't have the answer, search/inspect the codebase
3. **Web Search** - If code doesn't provide clarity, research online with proper citations
4. **Training Data** - Only as last resort to fill remaining gaps

**Always cite your source** - Tell the user which level you're answering from.

## When to Use

- **When user asks any question** - Check KB first before answering
- Encountering unfamiliar code patterns, services, or architecture
- When user asks questions outside training data that would require code research
- Before extensive codebase research—check KB first
- When explicitly asked to save or update knowledge
- When a topic has been researched and should be preserved

## Feeding Discoveries Back

After significant code exploration, consider what was learned:

1. **New knowledge**: Something not in the KB → offer to create entry
2. **Corrective knowledge**: Code contradicts KB → flag and offer to update
3. **Confirmatory knowledge**: Code confirms KB → no action needed (optionally note verification)

**If KB contradicts code**: Stop and report the discrepancy. Ask: "KB says X, but code shows Y - should I update the KB?" Do not silently ignore KB errors.

**End of exploration prompt**: "Did we discover anything worth preserving or correcting in the KB?"

## KB Location

```
~/doc/agent-kb/
```

## Querying the KB

1. List available entries:
   ```bash
   ls -1 ~/doc/agent-kb/
   ```

2. Search by tag or keyword:
   ```bash
   grep -l "pattern" ~/doc/agent-kb/*.md
   ```

3. Search by content before answering questions:
   ```bash
   grep -il "keyword" ~/doc/agent-kb/*.md
   ```

4. Read relevant entries before researching code.

## Creating KB Entries

See the **denote** skill for file naming convention and frontmatter format.

1. Generate timestamp: `date +%Y%m%dT%H%M%S`
2. Create file using Denote naming convention with `.md` extension
3. Add frontmatter (see denote skill for fields: `title`, `date`, `tags`, `identifier`, `verified`, `source`)
4. Keep entries focused—one topic per file.
5. **Link to other KB entries using `denote:{identifier}`** - e.g., "See denote:20260130T171856 for query details"
6. **Don't automatically follow cross-references** - Only load referenced documents if directly relevant to the current question.

## Staleness Handling

KB entries carry a `verified` date in frontmatter. Apply these thresholds when reading entries:

| Age | Action |
|-----|--------|
| Within 30 days | Trust entry, use directly |
| 31–90 days | Use with caution; flag as potentially stale |
| 90+ days | Verify against source code before acting; do not use blindly |

**When you confirm an entry is still accurate**: update the `verified` date in frontmatter.

**When you find an entry is wrong**: correct the content, update `verified`, and add a note on the last line: `Updated YYYY-MM-DD: <what changed>`.

## Updating KB Entries

When knowledge becomes stale or incomplete:
1. Read the existing entry
2. Update content in place
3. Preserve the original identifier and filename
4. **Update `verified` date** in frontmatter if re-confirmed against code
5. **Add `verified` and `source` fields** to frontmatter if missing from older entries

## What Belongs in the KB

- **Only verifiable knowledge from authoritative sources** - code inspection, official documentation, or properly cited research. No speculation, design alternatives, or "we could do X" proposals.
