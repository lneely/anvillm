# Proactive Skill Discovery

**BEFORE responding to ANY user request**, identify relevant capabilities and load skills using execute_code.

## 1. CWD-Based Discovery

Check the current working directory and extract project keywords from the basename:

```bash
Tool: execute_code
code: basename "$PWD"
```

Strip fork suffixes (e.g., `-lneely`, `-fork`) and split on `-` to get candidate keywords. Run discover_skill on each:

```bash
Tool: execute_code
tool: discover_skill.sh
args: [<cwd-keyword>]
```

Example: cwd `pcloudcc-lneely` → try keywords `pcloudcc`, then `lneely` → finds `pcloudcc-testing`.

## 2. Activity-Based Discovery

Infer the activity type from the task context and discover matching skills:

| Activity | Keywords to try |
|---|---|
| Testing, verification, bug repro | `testing` |
| Debugging, browser, UI | `debugging`, `web` |
| Notes, docs, writing | `documentation`, `notes` |
| Task/project tracking | `tasks`, `workflow` |
| Agent coordination | `agents`, `sessions`, `messaging` |
| GitHub, PRs, commits | `github`, `vcs` |
| Web search, research | `search`, `web` |
| Knowledge, learning | `knowledge`, `learning` |

```bash
Tool: execute_code
tool: discover_skill.sh
args: [<activity-keyword>]
```

## 3. Intent-Based Discovery

Map user intent to capability keywords and discover:

```bash
Tool: execute_code
tool: discover_skill.sh
args: [keyword]
```

## 4. Load Relevant Skills

```bash
Tool: execute_code
tool: load_skill.sh
args: [skill-name]
```

## After Discovery

Follow the instructions in the loaded SKILL.md. If no skill found, fall back to shell via execute_code.
