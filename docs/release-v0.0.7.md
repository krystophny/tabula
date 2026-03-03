# Release v0.0.7

> **Legal notice:** Tabura is provided "as is" and "as available" without warranties, and to the maximum extent permitted by applicable law the authors/contributors accept no liability for damages, data loss, or misuse. You are solely responsible for backups, verification, and safe operation. See [`DISCLAIMER.md`](/DISCLAIMER.md).

> Historical note: This file documents a past release. For the current runtime stack, use README.md and docs/architecture.md.

## Scope

`v0.0.7` introduces a two-column canvas layout, tap-to-reference artifact interaction, and live canvas auto-refresh via fsnotify when the agent edits files on disk.

## Highlights

### Two-Column Layout
- Desktop: document/artifact on the left, chat on the right (380px fixed width)
- Mobile (<768px): canvas is a full-screen overlay with close button
- When no artifact is open, chat takes full width

### Tap-to-Reference
- Right-click on artifact text sets a location context badge in the prompt bar (`Line N of "title"`)
- Long-press starts PTT voice recording with location context
- Text selection captures selected text as context
- Context is prepended to the chat message on send and cleared after

### Canvas Auto-Refresh (fsnotify)
- When an assistant turn starts, an fsnotify watcher is placed on the active artifact file
- File changes on disk are detected instantly via kernel inotify and pushed to the canvas via MCP
- Watches the parent directory to catch atomic write patterns (write temp + rename)
- Post-turn sync check as final safety net
- Forward non-agentMessage `item/completed` events from codex protocol as `item_completed` StreamEvents

### STT Fixes
- Firefox audio capture compatibility
- Whisper hallucination filter rejects silent-audio transcripts
- Client-side audio collection: one blob sent on stop instead of streaming chunks
- Elapsed time check removed (was rejecting all recordings)

### Canvas and Code Cleanup
- Extracted mail triage UI to `canvas-mail.js`
- Stripped annotation system (bubbles, side panel, thread keys)
- Removed `review-mode-workflow.md` and `server_commit_ai_test.go`
- Added `/clear` and `/compact` slash commands
- `text` accepted as fallback for `markdown_or_text` MCP parameter

## New Dependencies

- `github.com/fsnotify/fsnotify` v1.9.0

## Traceability

For publication metadata, associate this release with:

- release label: `v0.0.7`
- repository: `https://github.com/krystophny/tabura`
- exact source revision: tag target commit hash
