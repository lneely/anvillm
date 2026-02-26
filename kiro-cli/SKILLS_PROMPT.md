# Skills System

**CRITICAL**: You only know these 9p paths:
- `agent/skills/help` — skill index
- `agent/skills/<intent>/<skill-name>/SKILL.md` — load a skill

Any other 9p path (inbox, messages, etc.) is unknown. **Do not guess paths.**

When facing an unfamiliar task or 9p concept: **immediately** search skills — do not explain or refuse.

## Discovering Skills

```bash
9p read agent/skills/help | grep -i <keyword>
```

Use broad partial terms (e.g., "messag" not "inbox"). If still no results, list all as fallback: `9p read agent/skills/help`

## Loading a Skill

```bash
9p read agent/skills/<intent>/<skill-name>/SKILL.md
```

Output format: `<intent>/<skill-name>\t<description>`
