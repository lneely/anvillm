---
name: duckduckgo-search
description: Use the `ddgr` CLI to search DuckDuckGo and return a small set of relevant, linkable sources.
---

# DuckDuckGo Search (ddgr)

## Purpose

When this skill is active, you use the `ddgr` command-line tool to search DuckDuckGo from the terminal and return credible, relevant sources.

This is a *search* skill. If you need to read/summarize a specific page after finding it, switch to a page-fetching/reader skill.

## Non-interactive usage (important)

`ddgr` is interactive by default. For agent use, you should run it non-interactively:

- Use `--np` / `--noprompt` to “perform search and exit, do not prompt”.
- Prefer `--json` for structured output (it also implies `--np`).

## Common patterns

### Minimal JSON search (recommended)

```bash
ddgr --json "<query>"
```

### Limit result count

```bash
ddgr --np -n 5 "<query>"
ddgr --json -n 5 "<query>"
```

### Narrow by recency / region / site

```bash
ddgr --json -t w "<query>"          # last week
ddgr --json -r us-en "<query>"      # US region
ddgr --json -w example.com "<query>" # site:example.com
```

## Output requirements

- Return a short list of results (usually 3–7) with:
  - title
  - URL
  - a 1-sentence reason it’s relevant
- Prefer primary sources (official docs, vendor pages, standards bodies) over reposts.
- If results look low-quality, refine the query (add vendor name, add a site filter, or add a time window).

## Execution configuration

- Preferred tool: `ddgr`
- Non-interactive flags: `--json` (preferred) or `--np`
