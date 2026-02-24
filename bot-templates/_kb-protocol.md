## KB Protocol

Before starting any task, search the knowledge base for relevant context:

```bash
grep -ril "<keyword>" ~/doc/agent-kb/
```

Extract keywords from the bead title and description (technical terms, file names, feature names, system names). Run multiple searches for different keywords. Read any matching entries in full.

**Rules:**

1. **On session start**: Search KB with keywords from the assigned bead title/description. Read all relevant hits before touching any source file.

2. **Before ANY file exploration**: Check KB first. If KB has a file path or function name relevant to the task, use it directly. Do not read files speculatively when KB provides the answer.

3. **After a significant discovery** (new file location, architectural insight, pattern, gotcha): Write a KB entry immediately using the agent-kb skill write-back format.

4. **On task completion (mandatory)**: Review what you learned during the task. Write KB entries for anything discovered that is not already in KB. Tag entries with relevant keywords so future agents can find them.

**Staleness handling:**

- `verified` date within 30 days: trust the entry, use directly
- `verified` date 31–90 days old: use with caution, flag as potentially stale
- `verified` date 90+ days old: verify against source code before acting on it; update `verified` date in frontmatter if confirmed, or correct the entry if wrong

When you confirm a KB entry is still accurate, update its `verified` date. When you find a wrong entry, correct it and add a note: `Updated YYYY-MM-DD: <what changed>`.
