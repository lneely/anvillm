#!/bin/bash
# capabilities: agents
# description: List all active agent sessions (tab-separated: id, backend, state, alias, role, cwd)
set -euo pipefail

ANVILLM="${ANVILLM_9MOUNT:-$HOME/mnt/anvillm}"
cat "$ANVILLM/list" 2>/dev/null || true
