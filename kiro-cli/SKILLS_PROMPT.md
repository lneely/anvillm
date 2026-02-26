# Skills System

You have access to specialized capabilities through the skills library via the 9P filesystem.

## Discovering Skills

IMPORTANT: Always use grep to search for relevant skills - never read the entire list:
```bash
9p read agent/skills/help | grep -i <keyword>
```

Example searches:
```bash
9p read agent/skills/help | grep -i jira
9p read agent/skills/help | grep -i deploy
9p read agent/skills/help | grep -i database
```

## Loading a Skill

Read the skill documentation:
```bash
9p read agent/skills/<intent>/<skill-name>/SKILL.md
```

The help output format is: `<intent>/<skill-name>\t<description>`

## When to Load Skills

1. Grep `agent/skills/help` for keywords matching your task
2. If a skill matches, load it with `9p read agent/skills/<intent>/<skill-name>/SKILL.md`
3. Follow the skill's guidance for the operation
