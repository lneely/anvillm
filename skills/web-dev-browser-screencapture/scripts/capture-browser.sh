#!/usr/bin/env bash
# Capture browser window showing a specific URL
# Works on macOS (Chrome) and Linux/X11 (Firefox/Chrome via brotab)
# Usage: capture-browser.sh [output_path] [url_pattern]

set -e

OUTPUT="${1:-${TMPDIR:-/tmp}/browser_capture.png}"
URL_PATTERN="${2:-localhost}"

case "$(uname -s)" in
    Darwin)
        # macOS: use osascript + screencapture
        osascript -e 'tell application "Google Chrome" to activate' 2>/dev/null
        sleep 0.3
        osascript -e "tell application \"Google Chrome\" to set index of (first window whose name contains \"$URL_PATTERN\") to 1" 2>/dev/null
        sleep 0.3
        screencapture -x "$OUTPUT"
        ;;
    Linux)
        if [ -z "$DISPLAY" ]; then
            echo "ERROR: DISPLAY not set" >&2
            exit 1
        fi

        # Linux: use brotab + wmctrl + import
        TAB_INFO=$(brotab list | grep -i "$URL_PATTERN" | head -1)
        if [ -z "$TAB_INFO" ]; then
            echo "ERROR: No tab matching '$URL_PATTERN'" >&2
            exit 1
        fi

        TAB_ID=$(echo "$TAB_INFO" | cut -f1)
        TAB_TITLE=$(echo "$TAB_INFO" | cut -f2)
        echo "Found tab: $TAB_ID ($TAB_TITLE)" >&2

        brotab activate "$TAB_ID"
        sleep 0.5

        WIN_ID=$(wmctrl -l | grep -i "$TAB_TITLE" | head -1 | awk '{print $1}')
        if [ -z "$WIN_ID" ]; then
            WIN_ID=$(wmctrl -l | grep -iE "firefox|chrome|chromium" | head -1 | awk '{print $1}')
        fi

        if [ -z "$WIN_ID" ]; then
            echo "ERROR: Could not find browser window" >&2
            exit 1
        fi

        echo "Capturing window: $WIN_ID" >&2
        wmctrl -i -a "$WIN_ID"
        sleep 0.3
        import -window "$WIN_ID" "$OUTPUT"
        ;;
    *)
        echo "ERROR: Unsupported OS: $(uname -s)" >&2
        exit 1
        ;;
esac

if [ -f "$OUTPUT" ]; then
    echo "$OUTPUT"
else
    echo "ERROR: Failed to capture" >&2
    exit 1
fi
