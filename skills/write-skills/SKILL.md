---
name: write-skills
description: Guide for authoring new Agent Skills. Use when creating, updating, or reviewing skills / SKILL.md files. Always prefer the skill-creator skill if available.
---

# Write Skills (Meta Skill)

## Purpose

This skill provides guidance for authoring high-quality Agent Skills. Use it when creating new skills or improving existing ones.

## Skill Structure

Each skill is a directory containing:

```
my-skill/
├── SKILL.md          # Required - frontmatter + instructions
├── reference.md      # Optional - detailed documentation
├── examples.md       # Optional - usage examples
├── scripts/          # Optional - helper utilities
└── templates/        # Optional - file templates
```

## SKILL.md Format

```markdown
---
name: skill-name # must match the folder name of the skill!
description: Brief description of what this skill does and when to use it.
allowed-tools: Tool1, Tool2  # Optional - restrict available tools
---

# Skill Name

## Purpose
What the skill accomplishes.

## When to Use
Conditions that trigger this skill.

## Instructions
Step-by-step guidance for Agent.

## Examples
Concrete usage examples.
```

## Frontmatter Requirements

| Field | Rules |
|-------|-------|
| `name` | Lowercase letters, numbers, hyphens only. Max 64 chars. Match directory name. |
| `description` | Required. Max 1024 chars. Include *what* and *when*. |
| `allowed-tools` | Optional. Comma-separated tool names to restrict access. |

## Writing Effective Descriptions

The `description` field determines when Agent invokes your skill. Include:

1. **What** the skill does (capability)
2. **When** to use it (trigger conditions)

Good examples:
- "Use the `fetch-markdown` CLI tool to retrieve a URL and extract readable Markdown for quoting, summarization, and analysis."
- "Guide for authoring new Agent Skills. Use when creating, updating, or reviewing SKILL.md files."

Bad examples:
- "A useful skill" (too vague)
- "Helps with things" (no trigger condition)

## Writing Instructions

- Be clear and step-by-step
- Use imperative voice ("Do X", not "You should do X")
- Include "When to Use" vs "When NOT to Use" sections
- Provide concrete examples
- Reference companion files with relative paths: `[reference.md](reference.md)`

## Best Practices

1. **Single responsibility**: One skill, one purpose
2. **Progressive disclosure**: Put details in companion files, not SKILL.md
3. **Actionable guidance**: Tell Agent exactly what to do
4. **Clear boundaries**: Define when to use and when NOT to use
5. **Test your skill**: Verify Agent invokes it correctly

## Creating a New Skill

### 1. Find the skills directory

```bash
agent-skills path
```

This returns colon-separated paths. Use the first writable one (typically `~/src/agent-q/skills/`).

### 2. Create the skill directory and file

```bash
mkdir -p <skills-path>/<skill-name>
# Then create SKILL.md with the required format
```

### 3. Verify the skill is registered

```bash
agent-skills list | grep <skill-name>
```

### 4. Test loading

```bash
agent-skills load <skill-name>
```

## Skill Locations

| Type     | Path                    | Use Case                           |
|----------|-------------------------|------------------------------------|
| Agent-Q  | `~/src/agent-q/skills/` | Shared team skills                 |

## Restricting Tools

Use `allowed-tools` for read-only or limited-scope skills:

```yaml
allowed-tools: Read, Grep, Glob
```

When active, Agent should only use those tools without permission prompts.

## Validation Checklist

- [ ] Directory name matches `name` field
- [ ] `name` uses only `[a-z0-9-]`, max 64 chars
- [ ] `description` is present and under 1024 chars
- [ ] `description` explains what AND when
- [ ] Instructions are clear and actionable
- [ ] Companion files are referenced correctly
