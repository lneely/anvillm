---
name: Hotfix
description: Urgent bug fix that must be reviewed, tested, and deployed immediately
roles: developer, reviewer, tester, devops
---

Use this workflow for production bugs that require an immediate fix and
deployment. The full quality gate still applies — speed does not justify
skipping review or testing.

## Decomposition

| # | Title                     | Role      | Depends On |
|---|---------------------------|-----------|------------|
| 1 | Fix: <description>        | developer | —          |
| 2 | Review: <description>     | reviewer  | 1          |
| 3 | Verify fix: <description> | tester    | 2          |
| 4 | Deploy: <description>     | devops    | 3          |

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

**3 → 4 (tester approved → devops)**
Conductor includes in PROMPT_REQUEST:
- Test results summary
- Files modified
- Target environment and urgency context

## Notes

- After rework, the full review and test cycle repeats from step 2.
- Deployment should be expedited but must not bypass CI.
- If any required role is not present in the session, block and report to the user.
