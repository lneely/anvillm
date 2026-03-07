---
name: Incident
description: Production hotfix followed by post-mortem documentation
roles: developer, reviewer, tester, devops, author, technical-editor
---

Use this workflow for production incidents: fix and ship the hotfix, then
document what happened, why, and how it was resolved.

## Decomposition

| # | Title                     | Role             | Depends On |
|---|---------------------------|------------------|------------|
| 1 | Fix: <description>        | developer        | —          |
| 2 | Review: <description>     | reviewer         | 1          |
| 3 | Verify fix: <description> | tester           | 2          |
| 4 | Deploy: <description>     | devops           | 3          |
| 5 | Post-mortem: <description> | author          | 4          |
| 6 | Edit: <description>       | technical-editor | 5          |

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

**4 → 5 (devops → author)**
Conductor includes in PROMPT_REQUEST:
- Timeline of the incident and fix
- Root cause as identified by the developer
- Files modified and deployment details

**5 → 6 (author → technical-editor)**
Conductor includes in REVIEW_REQUEST:
- Files written or modified
- Post-mortem audience (internal team, stakeholders, etc.)
- Any sections the author flagged as uncertain

**6 rejected → 5 (technical-editor → author)**
Conductor includes in PROMPT_REQUEST:
- Editorial findings
- Locations and nature of each issue

## Notes

- After code rework, the full review and test cycle repeats from step 2.
- After post-mortem rework, the editorial review repeats from step 6.
- Deployment should be expedited but must not bypass CI.
- If any required role is not present in the session, block and report to the user.
