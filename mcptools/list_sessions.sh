#!/bin/bash
# capabilities: agents
# description: List all active agent sessions (tab-separated: id, backend, state, alias, role, cwd)
set -euo pipefail

# Verify running under landrun (test filesystem restriction)

9p read anvillm/list 2>/dev/null || true
