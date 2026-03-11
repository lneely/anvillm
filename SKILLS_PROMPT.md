# Proactive Skill Discovery

**BEFORE responding to ANY user request**, identify relevant capabilities and load skills using execute_code.

  MANDATORY FIRST STEP: Before ANY tool search, ALWAYS run discover_skill.sh
   with relevant keywords. Never use ToolSearch, Agent/Explore agents, or
  manual filesystem exploration to find tools. discover_skill.sh is the ONLY
   tool discovery mechanism.

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

# Code Exploration

**For code search and exploration**, use `code_explorer.sh` instead of raw find/grep. This reduces token usage via preview-then-fetch workflow.

**CRITICAL: Always use `sandbox: default` with code_explorer.sh** - without it, the tool runs in the wrong directory and indexes nothing.

## Workflow

1. Initialize index (once per worktree):
```bash
Tool: execute_code
tool: code_explorer.sh
args: [init]
sandbox: default
```

2. Preview matches (cheap - counts only):
```bash
Tool: execute_code
tool: code_explorer.sh
args: [preview, "<pattern>"]
sandbox: default
```

3. Fetch specific files (with context):
```bash
Tool: execute_code
tool: code_explorer.sh
args: [fetch, "<pattern>", "<file>"]
sandbox: default
```

4. Search symbols (if ctags index exists):
```bash
Tool: execute_code
tool: code_explorer.sh
args: [symbols, "<pattern>"]
sandbox: default
```

## Fallback

If code_explorer.sh is unavailable or errors, fall back to standard tools (grep, find, etc.). Empty results are valid - do not retry with other tools just because no matches were found.
