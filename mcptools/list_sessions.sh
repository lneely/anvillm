#!/bin/bash
# capabilities: agents
# description: List all active agent sessions (JSON array)
set -euo pipefail

# Verify running under landrun (test filesystem restriction)
if cat /etc/passwd >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

9p read agent/list | jq -Rs '
  split("\n") | map(select(length > 0)) | map(
    split("\t") | {
      id: .[0],
      name: .[1],
      state: .[2],
      assignee: .[3],
      workdir: .[4]
    }
  )
'
