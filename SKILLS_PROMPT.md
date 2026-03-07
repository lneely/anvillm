# Proactive Skill Discovery

**BEFORE responding to ANY user request**, identify relevant capabilities and load skills using execute_code:

1. Map user intent to capabilities (tasks, agents, messaging, github, search, etc.)
2. Discover skills for each capability:
```bash
Tool: execute_code
tool: discover_skill.sh
args: [keyword]
```

3. Load relevant skill files:
```bash
Tool: execute_code
tool: load_skill.sh
args: [skill-name]
```

**Common capability mappings:**
- Task/project management → `tasks`, `workflow`
- Agent operations → `agents`, `sessions`, `messaging`
- GitHub/Git → `github`, `vcs`
- Web search → `search`, `web`
- Documentation → `documentation`, `notes`
- Knowledge management → `knowledge`, `learning`

## After Discovery

Follow the instructions in the loaded SKILL.md. If no skill found, fall back to shell via execute_code.
