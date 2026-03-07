---
name: Feature with Documentation
description: Feature development followed by documentation of the new functionality
roles: developer, reviewer, tester, devops, author, technical-editor
---

Use this workflow for user-facing features where documentation must ship
alongside the code.

## Decomposition

| # | Title                    | Role             | Depends On |
|---|--------------------------|------------------|------------|
| 1 | Implement: <description> | developer        | —          |
| 2 | Review: <description>    | reviewer         | 1          |
| 3 | Test: <description>      | tester           | 2          |
| 4 | Deploy: <description>    | devops           | 3          |
| 5 | Write: <description>     | author           | 4          |
| 6 | Edit: <description>      | technical-editor | 5          |

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

**4 → 5 (devops → author)**
Conductor includes in PROMPT_REQUEST:
- Files modified
- Summary of the feature and its user-facing behavior
- Any deployment notes relevant to documentation

**5 → 6 (author → technical-editor)**
Conductor includes in REVIEW_REQUEST:
- Files written or modified
- Intended audience and purpose of the documentation
- Any sections the author flagged as uncertain

**6 rejected → 5 (technical-editor → author)**
Conductor includes in PROMPT_REQUEST:
- Editorial findings
- Locations and nature of each issue

## Notes

- After code rework, the full review and test cycle repeats from step 2.
- After doc rework, the editorial review repeats from step 6.
- Multiple implementation beads may be dispatched in parallel if they are orthogonal (no shared files). If two beads touch the same files, sequence them with a dependency instead.
- If any required role is not present in the session, block and report to the user.
