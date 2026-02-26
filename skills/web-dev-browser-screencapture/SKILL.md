---
name: web-dev-browser-screencapture
intent: web, debugging
description: Capture screenshots of browser windows for visual feedback during web development. Use when verifying UI changes, checking dev server output, or documenting web application state.
---

# Web Dev Browser Screencapture

Capture browser window showing a specific URL.

## Usage

```bash
capture_browser.sh [output_path] [url_pattern]
```

**Defaults:**
- `output_path`: `${TMPDIR:-/tmp}/browser_capture.png`
- `url_pattern`: `localhost`

**Examples:**
```bash
capture_browser.sh /tmp/capture.png localhost:3000
capture_browser.sh /tmp/app.png myapp.local
```

## Reading Captured Image

Use `fs_read` with Image mode:
```json
{"operations": [{"mode": "Image", "image_paths": ["<path_from_output>"]}]}
```

## Platform Support

- **macOS**: Google Chrome + `osascript` + `screencapture`
- **Linux**: `brotab` + `wmctrl` + `import` (ImageMagick)

## When to Use

- Verify UI changes match design
- Check dev server output visually
- Document application state
- Get visual feedback during development

## When NOT to Use

- Browser not running
- Target page not open
