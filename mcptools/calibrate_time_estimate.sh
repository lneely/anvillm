#!/bin/bash
# capabilities: estimation
# description: Calibrate time estimation model from completion data; auto-updates taxonomy when 'other' accumulates
# Usage: calibrate_time_estimate.sh
set -euo pipefail


TAXONOMY_FILE="$HOME/.config/anvillm/estimation/taxonomy.json"
MODEL_FILE="$HOME/.local/share/anvillm/estimation/model.json"
LOG_FILE="$HOME/.local/share/anvillm/estimation/completions.jsonl"
MIN_SAMPLES=3

if [ ! -f "$LOG_FILE" ]; then
    echo "No completion data at $LOG_FILE — nothing to calibrate."
    exit 0
fi

TOTAL=$(wc -l < "$LOG_FILE")
echo "Processing $TOTAL completion records..."

OTHER_THRESHOLD=$(jq -r '.other_threshold // 5' "$TAXONOMY_FILE")

# Compute per-domain median actual_seconds (only for domains with >= MIN_SAMPLES records)
NEW_BASE=$(jq -rs --argjson min "$MIN_SAMPLES" '
  [ .[] | select(.actual_seconds != null) ] |
  group_by(.domain) |
  map(select(length >= $min) | {
    key: .[0].domain,
    value: ( map(.actual_seconds) | sort | .[(length / 2 | floor)] )
  }) |
  from_entries
' "$LOG_FILE")

# Load or initialize model
if [ -f "$MODEL_FILE" ]; then
    CURRENT_MODEL=$(cat "$MODEL_FILE")
else
    CURRENT_MODEL='{"base_seconds": {}, "child_multiplier": 300, "desc_multiplier": 0.5}'
fi

# Merge updated base_seconds into model
echo "$CURRENT_MODEL" | jq --argjson new "$NEW_BASE" '.base_seconds += $new' > "$MODEL_FILE"
echo "Model updated: $MODEL_FILE"

# Check 'other' accumulation for taxonomy auto-update
if jq -e 'select(.domain == "other")' "$LOG_FILE" >/dev/null 2>&1; then
    # Count inferred_labels within 'other' records
    while IFS=$'\t' read -r COUNT LABEL; do
        [ -z "$LABEL" ] || [ "$LABEL" = "null" ] || [ "$LABEL" = "other" ] && continue
        if [ "$COUNT" -ge "$OTHER_THRESHOLD" ]; then
            EXISTS=$(jq -r --arg l "$LABEL" '.categories[] | select(. == $l)' "$TAXONOMY_FILE" 2>/dev/null || true)
            if [ -z "$EXISTS" ]; then
                echo "Auto-adding taxonomy category: $LABEL (${COUNT} samples)"
                jq --arg c "$LABEL" '.categories += [$c]' "$TAXONOMY_FILE" > "${TAXONOMY_FILE}.tmp"
                mv "${TAXONOMY_FILE}.tmp" "$TAXONOMY_FILE"
            fi
        fi
    done < <(jq -r 'select(.domain == "other") | .inferred_label // "null"' "$LOG_FILE" \
        | sort | uniq -c | sort -rn \
        | awk '{print $1"\t"$2}')
fi

echo "Calibration complete."
