---
name: write-skills
intent: meta, skills
description: Guide for authoring new Agent Skills. Use when creating, updating, or reviewing skills / SKILL.md files.
---

# Write Skills

## Structure

```
skill-name/
├── SKILL.md          # required: frontmatter + instructions
├── reference.md      # optional: detailed docs
└── examples.md       # optional: usage examples
```

## SKILL.md Format

```yaml
---
name: skill-name              # lowercase, hyphens, match directory
intent: category, subcategory # for discovery
description: What it does and when to use it (max 1024 chars)
---

# Skill Name

Brief, actionable instructions.
```

## Frontmatter

- `name` - `[a-z0-9-]`, max 64 chars, match directory
- `intent` - comma-separated categories (tasks, workflow, editor, messaging, etc.)
- `description` - what + when, max 1024 chars

## Writing Guidelines

**Description:** Include capability and trigger conditions.
- Good: "Manage tasks using beads 9P interface. Use when creating, updating, listing, or deleting tasks/beads."
- Bad: "A useful skill" (vague, no trigger)

**Instructions:**
- Imperative voice ("Do X" not "You should")
- Single responsibility
- Clear boundaries (when to use / not use)
- Concrete examples
- Reference companion files: `[reference.md](reference.md)`

## Creating Skills

```bash
# Find skills directory
9p ls anvillm/skills

# Create
mkdir -p ./skills/skill-name
# Write SKILL.md with frontmatter

# Verify
9p ls anvillm/skills | grep skill-name
```

## Validation

- [ ] Directory = `name` field
- [ ] `name` matches `[a-z0-9-]`, ≤64 chars
- [ ] `intent` present
- [ ] `description` present, ≤1024 chars, includes what + when
- [ ] Instructions clear and actionable
