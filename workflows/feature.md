---
name: Feature
description: Standard feature development with review, testing, and deployment
roles: developer, reviewer, tester, devops
---

Use this workflow for well-understood features where requirements are clear
and no upfront research is needed.

## Decomposition

| # | Title                    | Role      | Depends On |
|---|--------------------------|-----------|------------|
| 1 | Implement: <description> | developer | —          |
| 2 | Review: <description>    | reviewer  | 1          |
| 3 | Test: <description>      | tester    | 2          |
| 4 | Deploy: <description>    | devops    | 3          |

## Handoffs

**1 → 2 (developer → reviewer)**
Conductor includes in REVIEW_REQUEST:
- Files modified
- Summary of implementation approach
- Any deviations from the original bead description

**2 rejected → 1 (reviewer → developer)**
Conductor includes in PROMPT_REQUEST:
- Reviewer findings
- Files and locations requiring changes

**2 → 3 (reviewer approved → tester)**
Conductor includes in APPROVAL_REQUEST:
- Files modified
- Reviewer findings and resolution summary
- Acceptance criteria from the original bead

**3 failed → 1 (tester → developer)**
Conductor includes in PROMPT_REQUEST:
- Test failures and output
- Files under test

**3 → 4 (tester approved → devops)**
Conductor includes in PROMPT_REQUEST:
- Test results summary
- Files modified
- Any environment or configuration dependencies noted during testing

## Notes

- Multiple implementation beads may be dispatched in parallel if they are orthogonal (no shared files). If two beads touch the same files, sequence them with a dependency instead.
- After rework, the full review and test cycle repeats from step 2.
- Deployment typically means opening a PR and verifying CI passes.
- If any required role is not present in the session, block and report to the user.
