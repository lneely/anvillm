---
name: agent-kb
description: Query and maintain a local knowledge base of code insights and architecture notes. Use when encountering unfamiliar code, needing context beyond training, or when asked to save/update learned knowledge.
---

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

## File Naming Convention

```
YYYYMMDDThhmmss--title-in-lowercase-alphanumeric__tag1_tag2_tagN.md
```

- Timestamp: `date +%Y%m%dT%H%M%S`
- Title: lowercase, alphanumeric, hyphens only
- Tags: lowercase alphanumeric, underscore-separated, after double-underscore, no hyphens
  ```

Examples:
- `20260128T133522--example-flow__ref.md`
- `20260126T144226---architecture-notes__component1_component2_tag3.md`

## Creating KB Entries

1. Generate timestamp:
   ```bash
   date +%Y%m%dT%H%M%S
   ```

2. Create file with frontmatter matching filename:
   ```markdown
   ---
   title:      Title In Title Case
   date:       YYYY-MM-DD Day HH:MM
   tags:       [tag1, tag2]
   identifier: YYYYMMDDThhmmss
   verified:   YYYY-MM-DD
   source:     code-inspection | documentation | online | verbal | inferred
   ---

   # Title

   Content here.
   ```

   **Provenance fields:**
   - `verified`: Date last confirmed against code (update on confirmation)
   - `source`: How the knowledge was obtained
     - `code-inspection` - directly verified in source code
     - `documentation` - from internal docs (Confluence, etc.)
     - `online` - from external online documentation
     - `verbal` - expert opinion, discussion, tacit knowledge
     - `inferred` - derived from training data (lowest priority)

   If a source has a URL (online or documentation), add a `## Sources` section at the bottom of the document with the citation(s).

3. Keep entries focused—one topic per file.
4. **Link to other KB entries using `denote:{identifier}`** - e.g., "See denote:20260130T171856 for query details"
5. **Don't automatically follow cross-references** - Only load referenced documents if directly relevant to the current question.

## Updating KB Entries

When knowledge becomes stale or incomplete:
1. Read the existing entry
2. Update content in place
3. Preserve the original identifier and filename
4. **Update `verified` date** in frontmatter if re-confirmed against code
5. **Add `verified` and `source` fields** to frontmatter if missing from older entries

## What Belongs in the KB

- **Only verifiable knowledge from authoritative sources** - code inspection, official documentation, or properly cited research. No speculation, design alternatives, or "we could do X" proposals.
