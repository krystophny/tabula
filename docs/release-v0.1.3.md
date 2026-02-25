# Release v0.1.3

## Scope

`v0.1.3` focuses on PR review usability in canvas mode, model-aware prompt behavior, and stronger low-motion/e-ink readability while preserving the existing local-first runtime stack.

## Highlights

### PR Review in Canvas

- Added a GitHub-backed PR review mode routed through `/pr` with cumulative diff loading and tighter command intent handling.
- Improved markdown diff rendering in canvas with source-line anchoring and clearer changed-region visualization.
- Strengthened diff readability by making syntax-highlighted keywords render bold for code and diff views.

### Model-Aware Prompting

- Added model-specific system prompt hints via `ModelSystemHints(alias)`.
- Enabled Spark-specific delegation rules for higher-risk git conflict and recovery workflows.
- Threaded selected model alias through prompt builders so model hints are applied consistently per turn/history mode.

### Voice and Interaction Reliability

- Hardened voice stop/cancel handling across tap/command paths and improved long/noisy recording fallback behavior.
- Stabilized reconnect behavior by resuming app-server threads instead of replaying full history.
- Improved iOS edge interactions and silent-mode focus behavior for more predictable touch operation.

### Metadata and Versioning

- Bumped runtime metadata to `v0.1.3` in release-visible version fields.

## Traceability

For publication metadata, associate this release with:

- release label: `v0.1.3`
- repository: `https://github.com/krystophny/tabura`
- exact source revision: tag target commit hash
