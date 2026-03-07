---
name: Release
description: Package and ship already-completed, tested work
roles: devops
---

Use this workflow when development, review, and testing are complete and the
only remaining work is packaging, tagging, and deploying to production.

## Decomposition

| # | Title                   | Role   | Depends On |
|---|-------------------------|--------|------------|
| 1 | Release: <description>  | devops | —          |

## Handoffs

None. Output goes directly to the conductor and user.

## Notes

- This workflow assumes all quality gates have already been passed. If there
  is any doubt, use `hotfix` or `feature` instead.
- The devops bot should verify CI is green before releasing.
- If any required role is not present in the session, block and report to the user.
