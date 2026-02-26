#!/bin/bash
set -euo pipefail
# list_sessions - List all active agent sessions
# Usage: list_sessions
# Output: JSON array of sessions

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
