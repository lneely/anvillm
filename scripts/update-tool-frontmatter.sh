#!/bin/bash
# Update tool scripts with front-matter for capability-based discovery
set -euo pipefail

TOOLS_DIR="${HOME}/.config/anvillm/mcptools"

update_tool() {
    local file="$1"
    local caps="$2"
    local desc="$3"
    
    # Create temp file with new front-matter
    {
        echo "#!/bin/bash"
        echo "# capabilities: $caps"
        echo "# description: $desc"
        # Skip old header lines, keep rest
        tail -n +2 "$file" | grep -v "^# capabilities:" | grep -v "^# description:" | grep -v "^#.*Usage:" | grep -v "^# [a-z_]* -"
    } > "${file}.tmp"
    
    mv "${file}.tmp" "$file"
    chmod +x "$file"
}

# send_message.sh
update_tool "$TOOLS_DIR/send_message.sh" "messaging" "Send message to agent or user: FROM TO TYPE SUBJECT BODY"

# read_inbox.sh  
update_tool "$TOOLS_DIR/read_inbox.sh" "messaging" "Read agent inbox: AGENT_ID"

# list_sessions.sh
update_tool "$TOOLS_DIR/list_sessions.sh" "agents" "List all running agent sessions"

# set_state.sh
update_tool "$TOOLS_DIR/set_state.sh" "agents" "Set agent state: AGENT_ID STATE"

# list_skills.sh
update_tool "$TOOLS_DIR/list_skills.sh" "skills" "List available agent skills"

# test.sh
update_tool "$TOOLS_DIR/test.sh" "testing" "Test tool for debugging"

echo "Updated front-matter for all tools"
