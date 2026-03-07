---
name: Documentation (Research Required)
description: Documentation that requires technical investigation before writing
roles: researcher, author, technical-editor
---

Use this workflow when documentation cannot be written without first
researching the subject — e.g. a new architecture guide, an unfamiliar
API's reference docs, or documenting a system that is not yet well understood.

## Decomposition

| # | Title                  | Role             | Depends On |
|---|------------------------|------------------|------------|
| 1 | Research: <description> | researcher      | —          |
| 2 | Write: <description>   | author           | 1          |
| 3 | Edit: <description>    | technical-editor | 2          |

## Handoffs

**1 → 2 (researcher → author)**
Conductor includes in PROMPT_REQUEST:
- Research findings
- Relevant file paths, APIs, or external references
- Intended audience and purpose of the documentation

**2 → 3 (author → technical-editor)**
Conductor includes in REVIEW_REQUEST:
- Files written or modified
- Intended audience and purpose of the documentation
- Any sections the author flagged as uncertain

**3 rejected → 2 (technical-editor → author)**
Conductor includes in PROMPT_REQUEST:
- Editorial findings
- Locations and nature of each issue

## Notes

- After rework, the editorial review repeats from step 3.
- If any required role is not present in the session, block and report to the user.
