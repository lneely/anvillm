---
name: Research
description: Standalone research or investigation task with no implementation
roles: researcher
---

Use this workflow when the goal is purely to answer a question, evaluate an
approach, or produce a findings report — with no coding deliverable.

## Decomposition

| # | Title                   | Role       | Depends On |
|---|-------------------------|------------|------------|
| 1 | Research: <description> | researcher | —          |

## Notes

- The output of the research bead should be a written summary delivered to
  the conductor, which passes it to the user.
- If the findings suggest follow-on implementation work, the conductor should
  report that to the user rather than spawning additional beads unilaterally.
- If any required role is not present in the session, block and report to the user.
