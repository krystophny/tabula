# Release v0.1.4

> **Legal notice:** Tabura is provided "as is" and "as available" without warranties, and to the maximum extent permitted by applicable law the authors/contributors accept no liability for damages, data loss, or misuse. You are solely responsible for backups, verification, and safe operation. See [`DISCLAIMER.md`](/DISCLAIMER.md).

## Scope

`v0.1.4` consolidates post-`v0.1.3` interaction updates with a focus on voice-capture consistency, model/runtime controls, and mobile interaction reliability.

## Highlights

### Voice Capture and Context Anchoring

- Unified push-to-talk start handling for tap, mouse hold, touch hold, and Ctrl long-press through one anchor-aware path.
- Anchored Ctrl push-to-talk to the latest cursor position so artifact context prefixes resolve consistently with tap-based capture.
- Hardened stop/flush and hidden-tab reconnect behavior to recover voice capture more reliably.

### Model and Session Controls

- Added per-project main chat model selection.
- Added reasoning-effort selection support including `extra_high` for Spark profile paths.
- Improved persistence and runtime handling for model/session profile behavior.

### Silent/Chat Interaction and UI Reliability

- Added top-bar silent toggle behavior and refined silent-mode response handling.
- Added chat-pane voice tap, text input, and push-to-talk interaction support.
- Refined recording cues and edge interactions across mobile/iOS standalone contexts for clearer, more stable feedback.

### Metadata and Versioning

- Bumped runtime metadata to `v0.1.4` in release-visible version fields.

## Traceability

For publication metadata, associate this release with:

- release label: `v0.1.4`
- repository: `https://github.com/krystophny/tabura`
- exact source revision: tag target commit hash
