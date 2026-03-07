---
name: Feature (Research Required)
description: Complex feature where design or technical approach must be established before implementation
roles: researcher, developer, reviewer, tester, devops
---

Use this workflow when the implementation approach is unclear, the feature
touches unfamiliar territory, or the bead description lacks sufficient detail
to begin coding directly.

## Decomposition

| # | Title                    | Role       | Depends On |
|---|--------------------------|------------|------------|
| 1 | Research: <description>  | researcher | —          |
| 2 | Implement: <description> | developer  | 1          |
| 3 | Review: <description>    | reviewer   | 2          |
| 4 | Test: <description>      | tester     | 3          |
| 5 | Deploy: <description>    | devops     | 4          |

## Handoffs

**1 → 2 (researcher → developer)**
Conductor includes in PROMPT_REQUEST:
- Research findings and recommendation
- Chosen approach and rationale
- Relevant file paths, APIs, or external references

**2 → 3 (developer → reviewer)**
Conductor includes in REVIEW_REQUEST:
- Files modified
- Summary of implementation approach
- Any deviations from the research recommendation

**3 rejected → 2 (reviewer → developer)**
Conductor includes in PROMPT_REQUEST:
- Reviewer findings
- Files and locations requiring changes

**3 → 4 (reviewer approved → tester)**
Conductor includes in APPROVAL_REQUEST:
- Files modified
- Reviewer findings and resolution summary
- Acceptance criteria from the original bead

**4 failed → 2 (tester → developer)**
Conductor includes in PROMPT_REQUEST:
- Test failures and output
- Files under test

**4 → 5 (tester approved → devops)**
Conductor includes in PROMPT_REQUEST:
- Test results summary
- Files modified
- Any environment or configuration dependencies noted during testing

## Notes

- Multiple implementation beads may be dispatched in parallel if they are orthogonal (no shared files). If two beads touch the same files, sequence them with a dependency instead.
- After rework, the full review and test cycle repeats from step 3.
- The research bead should produce a concrete recommendation or design
  decision, not an open-ended exploration. Scope it accordingly.
- If research reveals the feature is larger than expected, escalate to
  the user before proceeding to implementation.
- If any required role is not present in the session, block and report to the user.
