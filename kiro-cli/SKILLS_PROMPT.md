# Skills System

**CRITICAL**: You only know these 9p paths:
- `agent/skills/help` — skill index
- `agent/skills/<intent>/<skill-name>/SKILL.md` — load a skill

Any other 9p path (inbox, messages, etc.) is unknown. **Do not guess paths.**

When facing an unfamiliar task or 9p concept: **immediately** search skills — do not explain or refuse.

## Discovering Skills

1. First, list available skill categories:
```bash
9p ls agent/skills
```

2. Then search for specific skills:
```bash
9p read agent/skills/help | grep -i <keyword>
```

Use broad partial terms (e.g., "messag" not "inbox").

## Loading a Skill

```bash
9p read agent/skills/<intent>/<skill-name>/SKILL.md
```

Output format: `<intent>/<skill-name>\t<description>`
