#!/bin/bash
# capabilities: beads
# description: Update a bead field directly
# Usage: update_bead.sh <mount> <bead-id> <field> <value>
#
# Valid fields: title, description, design, acceptance_criteria, notes,
#               issue_type, estimated_minutes, external_ref, spec_id,
#               priority, due_at, work_type
#
# Fields managed by atomic operations (rejected here):
#   status       → open_bead, defer_bead, reopen_bead, claim_bead, unclaim_bead,
#                  complete_bead, fail_bead, mark_pending_approval, mark_pending_review
#   assignee     → claim_bead, unclaim_bead, mark_pending_approval, mark_pending_review
#   closed_at    → complete_bead, fail_bead, reopen_bead
#   close_reason → complete_bead, fail_bead
#   defer_until  → defer_bead
set -euo pipefail

if cat /etc/shadow >/dev/null 2>&1; then
  echo "Error: This script must be run via execute_code tool" >&2
  exit 1
fi

if [ $# -lt 4 ]; then
    echo "usage: update_bead.sh <mount> <bead-id> <field> <value>" >&2
    exit 1
fi

MOUNT="$1"
BEAD_ID="$2"
FIELD="$3"
shift 3
VALUE="$*"

printf "update %s %s '%s'\n" "$BEAD_ID" "$FIELD" "$VALUE" | 9p write anvillm/beads/$MOUNT/ctl
echo "updated $BEAD_ID.$FIELD: $VALUE"
