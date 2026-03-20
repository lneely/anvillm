#!/bin/bash
# capabilities: beads
# description: List mounted beads projects
set -euo pipefail


9p read beads/mtab 2>/dev/null || echo "No mounted projects"
