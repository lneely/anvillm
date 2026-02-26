---
name: web-dev-browser-screencapture
intent: web, debugging
description: Capture screenshots of browser windows for visual feedback during web development. Use when verifying UI changes, checking dev server output, or documenting web application state.
---

# Web Dev Browser Screencapture

Capture browser window showing a specific URL.

## Commands

- Capture localhost: `capture_browser.sh /tmp/capture.png localhost:<port>`
- Capture custom URL: `capture_browser.sh /tmp/app.png <url_pattern>`
- Capture default (localhost): `capture_browser.sh /tmp/capture.png`

## Reading Captured Image

```json
{"operations": [{"mode": "Image", "image_paths": ["<path_from_output>"]}]}
```

## Platform Support

- **macOS**: Chrome + `osascript` + `screencapture`
- **Linux**: `brotab` + `wmctrl` + `import` (ImageMagick)

## When to Use

- Verify UI changes
- Check dev server output
- Document application state

## When NOT to Use

- Browser not running
- Target page not open
