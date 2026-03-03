# Release v0.0.9

> **Legal notice:** Tabura is provided "as is" and "as available" without warranties, and to the maximum extent permitted by applicable law the authors/contributors accept no liability for damages, data loss, or misuse. You are solely responsible for backups, verification, and safe operation. See [`DISCLAIMER.md`](/DISCLAIMER.md).

> Historical note: This file documents a past release. For the current runtime stack, use README.md and docs/architecture.md.

## Scope

`v0.0.9` stabilizes the chat/canvas pipeline around a single file-backed canvas model, improves delegated Codex job lifecycle handling, and hardens runtime/CI reliability for this experimental phase.

## Highlights

### Chat + Canvas Simplification

- Enforced one canonical canvas path: file-backed artifacts via `:::file{...}`.
- Added automatic long-response routing to temp canvas files when model output is multi-paragraph.
- Suppressed duplicated chat/TTS output when content is routed to canvas-only mode.
- Removed/trimmed misleading legacy pathways so Spark receives one clear behavior model.

### Delegation Runtime Improvements

- Added asynchronous delegation controls and activity endpoints:
  - `delegate_to_model_status`
  - `delegate_to_model_cancel`
  - `delegate_to_model_active_count`
  - `delegate_to_model_cancel_all`
- Added split stop semantics in UI/backend (direct work vs delegated jobs).
- Added backend delegation diagnostics so MCP/tool failures are visible in logs.

### Context and Session Hygiene

- `/clear` now performs a full context reset:
  - active/queued turns
  - delegated jobs
  - temp canvas files
  - persisted chat messages/events
  - app-server threads/sessions
- Added prompt-contract digest guard:
  - if prompt contract changes, contexts are reset automatically.
  - first-run digest initialization does not clear existing messages.

### Voice and Frontend Behavior

- Prevented canvas output from being spoken by TTS.
- Improved indicator/overlay transitions between tabula rasa and artifact mode.
- Added regression tests for zen-canvas behavior and auto-canvas routing.

### Tooling and CI

- Added prompt-contract CI check script and workflow step.
- Fixed CI portability for prompt-contract checks on runners without `rg` by falling back to `grep`.
- Synced generated surface docs (`AGENTS.md`, `docs/interfaces.md`) with tool surface changes.

## Traceability

For publication metadata, associate this release with:

- release label: `v0.0.9`
- repository: `https://github.com/krystophny/tabura`
- exact source revision: tag target commit hash
