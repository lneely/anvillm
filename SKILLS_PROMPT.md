# CRITICAL: AnviLLM Discovery Protocol

**FORBIDDEN**: Raw 9p commands (9p read, 9p write, 9p ls) for AnviLLM operations.

**REQUIRED**: Use discovered tools with execute_code tool. Tool output is always correct.

For ANY AnviLLM task (sessions, messages, beads, agents, etc.):

1. Search for a tool using execute_code:
```
Tool: execute_code
Language: bash
Code:
bash <(9p read agent/tools/mcp/discover_tool.sh) <keyword>
```

2. If no tool, search for a skill using execute_code:
```
Tool: execute_code
Language: bash
Code:
bash <(9p read agent/tools/mcp/discover_skill.sh) <keyword>
```

3. Execute what you found using execute_code. If nothing found, tell the user.

Keywords: "session", "message", "bead", "agent", "inbox", "mail"

## After Discovery

Execute tool using execute_code:
```
Tool: execute_code
Language: bash
Code:
bash <(9p read agent/tools/<capability>/<tool-name>.sh) [args...]
```

**IMPORTANT**: All bead tools require `<mount>` as first arg (e.g., `label_bead.sh anvillm anv-123 claimable`, `read_bead.sh anvillm anv-123 json`). Only exceptions: `mount_beads.sh` (takes cwd), `umount_beads.sh` (takes name), `list_mounts.sh` (no args).

**Trust the tool output. Never use raw 9p commands as verification or fallback.**
