#!/bin/bash
# capabilities: agents
# description: List all active agent sessions (JSON array)
set -euo pipefail

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
