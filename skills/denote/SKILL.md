---
name: denote
intent: notes, documentation, knowledge
description: Denote file format conventions for timestamped notes. Use when creating, naming, or linking structured note files in the ~/doc/ hierarchy.
---

# Denote File Format

Denote is an Emacs structured note-taking system by Protesilaos Stavrou. It uses a strict filename convention to encode metadata.

## File Naming Convention

```
YYYYMMDDThhmmss--title-in-lowercase-alphanumeric__tag1_tag2_tagN.ext
```

Components:
- **Timestamp**: `date +%Y%m%dT%H%M%S` — ISO 8601 compact, seconds precision
- **Title**: lowercase, alphanumeric characters and hyphens only (no spaces, no underscores)
- **Tags**: lowercase alphanumeric, underscore-separated, after double-underscore. **CRITICAL**: Never use hyphens, underscores, or special characters in tag frontmatter!

Examples:
- `20260128T133522--example-flow__ref.md`
- `20260126T144226--architecture-notes__component1_component2_tag3.md`

## Frontmatter Format

Every Denote file begins with YAML frontmatter that mirrors the filename:

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

**Provenance fields** (for agent-kb entries):
- `verified`: Date last confirmed against code (update on confirmation)
- `source`: How the knowledge was obtained
  - `code-inspection` — directly verified in source code
  - `documentation` — from internal docs
  - `online` — from external online documentation
  - `verbal` — expert opinion, discussion, tacit knowledge
  - `inferred` — derived from training data (lowest priority)

If a source has a URL, add a `## Sources` section at the bottom.

## Cross-References

Link to other Denote entries using the identifier:

```
See denote:20260130T171856 for query details.
```

**Do not automatically follow cross-references.** Only load referenced documents if directly relevant to the current question.

## Looking Up an Entry by Identifier

```bash
grep -r "20260130T171856" ~/doc/agent-kb/
```

Or search by keyword:

```bash
grep -il "keyword" ~/doc/agent-kb/*.md
```
