---
name: Refactor
description: Structural code improvement with no behavior change
roles: developer, reviewer
---

Use this workflow for refactoring work: restructuring, renaming, extracting
abstractions, improving readability. No new behavior is introduced, so no
formal testing pass or deployment step is required.

## Decomposition

| # | Title                      | Role      | Depends On |
|---|----------------------------|-----------|------------|
| 1 | Refactor: <description>    | developer | —          |
| 2 | Review: <description>      | reviewer  | 1          |

## Handoffs

**1 → 2 (developer → reviewer)**
Conductor includes in REVIEW_REQUEST:
- Files modified
- Description of structural changes made
- Confirmation that no behavior was changed and existing tests still pass

**2 rejected → 1 (reviewer → developer)**
Conductor includes in PROMPT_REQUEST:
- Reviewer findings
- Files and locations requiring changes

## Notes

- After rework, review repeats from step 2.
- Multiple implementation beads may be dispatched in parallel if they are orthogonal (no shared files). If two beads touch the same files, sequence them with a dependency instead.
- If the refactor reveals bugs or missing tests, those should be tracked as
  separate beads rather than folded into this workflow.
- If any required role is not present in the session, block and report to the user.
