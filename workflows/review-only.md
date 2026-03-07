---
name: Review Only
description: Review and verify existing code with no implementation step
roles: reviewer, tester
---

Use this workflow when code already exists and only needs to be reviewed and
verified — e.g. an external PR, a branch written outside the system, or
work submitted for a second opinion.

## Decomposition

| # | Title                    | Role     | Depends On |
|---|--------------------------|----------|------------|
| 1 | Review: <description>    | reviewer | —          |
| 2 | Test: <description>      | tester   | 1          |

## Handoffs

**1 → 2 (reviewer approved → tester)**
Conductor includes in APPROVAL_REQUEST:
- Files under review
- Reviewer findings and resolution summary
- Acceptance criteria or context from the original bead

**1 rejected → conductor**
Conductor reports findings to the user. No testing is performed on rejected code.

## Notes

- If review rejects, the workflow ends. It is the user's responsibility to
  arrange rework and resubmit.
- If any required role is not present in the session, block and report to the user.
