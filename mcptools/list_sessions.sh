#!/bin/bash
# capabilities: agents
# description: List all active agent sessions (tab-separated: id, backend, state, alias, role, cwd)
set -euo pipefail

# Verify running under landrun (test filesystem restriction)
if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

9p read agent/list 2>/dev/null || true
