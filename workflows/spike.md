---
name: Spike
description: Time-boxed exploratory investigation to determine whether and how to proceed
roles: researcher
---

Use this workflow when the goal is to evaluate an approach, assess feasibility,
or answer a strategic question — not to produce a design or implementation.
The output is a recommendation: proceed, don't proceed, or proceed differently.

## Decomposition

| # | Title                  | Role       | Depends On |
|---|------------------------|------------|------------|
| 1 | Spike: <description>   | researcher | —          |

## Handoffs

None. Output goes directly to the conductor and user.

## Notes

- The researcher must time-box exploration. If the answer cannot be determined
  within reasonable effort, that itself is the finding — report it and stop.
- The output must include a clear recommendation, not an open-ended summary.
- If the recommendation is to proceed, the conductor should report this to the
  user and wait for a follow-on bead rather than spawning implementation work
  unilaterally.
- If any required role is not present in the session, block and report to the user.
