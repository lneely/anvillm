#!/bin/bash
# set_state - Set agent state
# Usage: set_state <agent_id> <state>
# Valid states: idle, running, stopped, starting, error, exited

agent_id="$1"
state="$2"

case "$state" in
  idle|running|stopped|starting|error|exited)
    echo "$state" | 9p write "agent/${agent_id}/state"
    ;;
  *)
    echo "Invalid state: $state" >&2
    echo "Valid states: idle, running, stopped, starting, error, exited" >&2
    exit 1
    ;;
esac
