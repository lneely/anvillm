#!/bin/bash
# list_sessions - List all active agent sessions
# Usage: list_sessions
# Output: JSON array of sessions

sessions=()
while IFS=$'\t' read -r id name state assignee workdir; do
  sessions+=("{\"id\":\"$id\",\"name\":\"$name\",\"state\":\"$state\",\"assignee\":\"$assignee\",\"workdir\":\"$workdir\"}")
done < <(9p read agent/list)

echo "[$(IFS=,; echo "${sessions[*]}")]"
