---
name: Bug Fix
description: Minimal workflow for a well-understood, self-contained bug fix
roles: developer, reviewer, tester
---

Use this workflow for bugs where the cause is already known and the fix is
straightforward. No research, no deployment step.

## Decomposition

| # | Title                     | Role      | Depends On |
|---|---------------------------|-----------|------------|
| 1 | Fix: <description>        | developer | —          |
| 2 | Review: <description>     | reviewer  | 1          |
| 3 | Verify fix: <description> | tester    | 2          |

## Handoffs

**1 → 2 (developer → reviewer)**
Conductor includes in REVIEW_REQUEST:
- Files modified
- Description of the fix and root cause
- Any edge cases or areas of concern

**2 rejected → 1 (reviewer → developer)**
Conductor includes in PROMPT_REQUEST:
- Reviewer findings
- Files and locations requiring changes

**2 → 3 (reviewer approved → tester)**
Conductor includes in APPROVAL_REQUEST:
- Files modified
- Description of the bug and expected correct behavior
- Reviewer findings and resolution summary

**3 failed → 1 (tester → developer)**
Conductor includes in PROMPT_REQUEST:
- Test failures and reproduction steps
- Files under test

## Notes

- The tester's job here is specifically to verify the bug is fixed, not
  exhaustive regression testing.
- After rework, the full review and test cycle repeats from step 2.
- If the fix turns out to be non-trivial during implementation, escalate to
  the `feature` workflow.
- If any required role is not present in the session, block and report to the user.
