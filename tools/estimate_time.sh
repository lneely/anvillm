#!/bin/bash
# capabilities: estimation
# description: Estimate LLM task completion time from bead signals
# Usage: estimate_time.sh '<json>'
# Input JSON: {"domain": "api", "child_bead_count": 3, "description_length": 450, "inferred_label": "api"}
# Output JSON: {"estimate_seconds": 1800, "tier": "moderate", "deadline_seconds": 3600}
set -euo pipefail


if [ $# -lt 1 ]; then
    echo "usage: estimate_time.sh '<json-signals>'" >&2
    exit 1
fi

TAXONOMY_FILE="$HOME/.config/anvillm/estimation/taxonomy.json"
MODEL_FILE="$HOME/.local/share/anvillm/estimation/model.json"

INPUT="$1"

# Extract signals
DOMAIN=$(echo "$INPUT" | jq -r '.domain // "other"')
CHILD_COUNT=$(echo "$INPUT" | jq -r '.child_bead_count // 0')
DESC_LEN=$(echo "$INPUT" | jq -r '.description_length // 0')

# Validate domain against taxonomy; fall back to "other" if unrecognized
if [ -f "$TAXONOMY_FILE" ]; then
    VALID=$(jq -r --arg d "$DOMAIN" '.categories[] | select(. == $d)' "$TAXONOMY_FILE" 2>/dev/null || true)
    if [ -z "$VALID" ]; then
        DOMAIN="other"
    fi
fi

# Load fitted constants from model.json, or use cold-start defaults
if [ -f "$MODEL_FILE" ]; then
    BASE=$(jq -r --arg d "$DOMAIN" '.base_seconds[$d] // 1200' "$MODEL_FILE")
    CHILD_MULT=$(jq -r '.child_multiplier // 300' "$MODEL_FILE")
    DESC_MULT=$(jq -r '.desc_multiplier // 0.5' "$MODEL_FILE")
else
    case "$DOMAIN" in
        ui)           BASE=600  ;;
        api)          BASE=900  ;;
        data-model)   BASE=1200 ;;
        infra)        BASE=1500 ;;
        testing)      BASE=600  ;;
        refactor)     BASE=1800 ;;
        docs)         BASE=300  ;;
        security)     BASE=1800 ;;
        performance)  BASE=1500 ;;
        research)     BASE=1200 ;;
        *)            BASE=1200 ;;
    esac
    CHILD_MULT=300
    DESC_MULT=0.5
fi

# Compute estimate and deadline (2x estimate)
ESTIMATE=$(awk -v base="$BASE" -v cc="$CHILD_COUNT" -v cm="$CHILD_MULT" -v dl="$DESC_LEN" -v dm="$DESC_MULT" \
    'BEGIN { printf "%d", base + (cc * cm) + (dl * dm) }')
DEADLINE=$(awk -v e="$ESTIMATE" 'BEGIN { printf "%d", e * 2 }')

# Classify tier
if   [ "$ESTIMATE" -lt 300 ];  then TIER="trivial"
elif [ "$ESTIMATE" -lt 1800 ]; then TIER="moderate"
elif [ "$ESTIMATE" -lt 7200 ]; then TIER="large"
else                                 TIER="very_large"
fi

jq -n \
    --argjson est "$ESTIMATE" \
    --arg tier "$TIER" \
    --argjson dl "$DEADLINE" \
    '{"estimate_seconds": $est, "tier": $tier, "deadline_seconds": $dl}'
