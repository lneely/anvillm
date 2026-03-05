# CRITICAL: AnviLLM Discovery Protocol

## Proactive Role Discovery

REQUIRED FIRST STEP: **If assigned a role or identity**, using patterns such as "you're a(n)...", "you are a(n)...", "act as...", "be a(n)...", discover and load the appropriate role and adopt it if found. Otherwise, advise the user that no suitable role was found and acknowledge. Use the `execute_code` (language: bash) tool to:

1. Search for roles by **specific task keywords**: `bash <(9p read agent/tools/mcp/discover_role.sh) <specific-keyword>`
2. Load role content: `9p read agent/roles/<focus-area>/<role-filename>.md`
3. Adopt the role's persona and expertise for your response

## Proactive Skill Discovery

REQUIRED NEXT STEP: Identify user intents, and load relevant skills:

1. Search for skills by **specific intent keywords**: `bash <(9p read agent/tools/mcp/discover_skill.sh) <specific-keyword>`
2. Load relevant skill files: `9p read agent/skills/<intent>/<skill-name>/SKILL.md`

## Tool Discovery

Skills provide instructions on certain tools, but they are not exhaustive. Identify other useful tools by:

1. Mapping user intent to capabilities
2. Search for tools by **specific capability keywords**: `bash <(9p read agent/tools/mcp/discover_tool.sh) <specific-keyword>`

Keep these tools and their usage in your memory for later use, when appropriate.
