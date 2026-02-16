---
name: web-dev-browser-screencapture
description: Capture screenshots of browser windows for visual feedback during web development. Use when verifying UI changes, checking dev server output, or documenting web application state.
---

# Web Dev Browser Screencapture

## Purpose

Capture screenshots of browser windows showing localhost dev servers for visual feedback.

## When to Use

- Verifying UI changes match expected design
- Checking dev server output visually
- Documenting current state of web application
- Getting visual feedback during development

## When NOT to Use

- When browser is not running
- When target page is not open in browser

## Instructions

### Usage

```bash
<capture-browser.sh> ${TMPDIR:-/tmp}/capture.png localhost:3000
```

If running in a sandbox without display access, use `execute-elevated-bash` (no quotes):

```bash
execute-elevated-bash <capture-browser.sh> ${TMPDIR:-/tmp}/capture.png localhost:3000
```

Note: Replace `<capture-browser.sh>` with the actual path shown in "Companion Files" above.

### Parameters

- First arg: output path (default: `${TMPDIR:-/tmp}/browser_capture.png`)
- Second arg: URL pattern to match (default: `localhost`)

### Reading the captured image

Use `fs_read` with Image mode:

```json
{"operations": [{"mode": "Image", "image_paths": ["<path_from_script_output>"]}]}
```

Then describe the UI elements visible.

## Platform Support

The script auto-detects the OS via `uname -s`:

**macOS:**
- Uses `osascript` to activate Chrome and bring matching window to front
- Uses `screencapture` for capture
- Requires: Google Chrome

**Linux (X11):**
- Uses `brotab` to find and activate the tab by URL
- Uses `wmctrl` to find window by page title
- Uses `import` (ImageMagick) for capture
- Requires: brotab with browser extension, wmctrl, ImageMagick
