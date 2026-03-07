---
name: Documentation
description: Write and editorially review documentation
roles: author, technical-editor
---

Use this workflow for any documentation task: new guides, reference material,
API docs, README updates, or revisions to existing content.

## Decomposition

| # | Title                | Role             | Depends On |
|---|----------------------|------------------|------------|
| 1 | Write: <description> | author           | —          |
| 2 | Edit: <description>  | technical-editor | 1          |

## Handoffs

**1 → 2 (author → technical-editor)**
Conductor includes in REVIEW_REQUEST:
- Files written or modified
- Intended audience and purpose of the documentation
- Any sections the author flagged as uncertain

**2 rejected → 1 (technical-editor → author)**
Conductor includes in PROMPT_REQUEST:
- Editorial findings
- Locations and nature of each issue

## Notes

- After rework, the editorial review repeats from step 2.
- If the author needs technical details from the codebase, that is within scope
  of the author role and does not require a separate research bead.
- If any required role is not present in the session, block and report to the user.
