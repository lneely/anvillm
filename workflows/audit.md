---
name: Audit
description: Code quality assessment of existing code with no changes made
roles: reviewer
---

Use this workflow to assess the quality, correctness, or security posture of
existing code without making any changes. Output is a findings report.

## Decomposition

| # | Title                  | Role     | Depends On |
|---|------------------------|----------|------------|
| 1 | Audit: <description>   | reviewer | —          |

## Handoffs

None. Findings go directly to the conductor and user.

## Notes

- The reviewer must not make any changes to the codebase.
- The output should be a structured findings report, not a pass/fail verdict.
- If findings warrant fixes, track them as separate beads using an appropriate
  workflow rather than folding them into this one.
- If any required role is not present in the session, block and report to the user.
