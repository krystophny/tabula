# Release v0.1.1

## Scope

`v0.1.1` focuses on mobile reliability and interaction consistency across silent and chat modes, with additional platform-specific polish.

## Highlights

### iOS Visual Cue and Recording State Polishing

- Reworked the recording/working edge-frame cue to track iOS full-screen behavior more reliably.
- Added full-screen safe behavior for standalone/web app mode with stronger mobile visual fidelity.
- Tightened bottom-corner rounding treatment so the active state aligns with iPhone rounded hardware edges.

### Control Reliability in Silent Chat

- Fixed mobile stop/working-state controls to remain tappable in silent mode (including iPhone chat-pane/edge states).
- Stabilized recorder and in-flight work cancel paths to reduce missed-stop behavior after longer sessions.

### Reasoning Effort Consistency

- Added and normalized `extra_high` reasoning effort handling in the UI model selector/payload path.
- Made model profile behavior consistent across available model families for effort-aware options.

### Metadata and Versioning

- Bumped runtime metadata to `v0.1.1` in release-visible version fields.

### Prompt and Delegation Defaults

- Centralized default Codex instruction handling with `git` and `gh` as preferred repository/PR workflows.
- Made default prompt templates configurable by output mode (`voice`/`silent`) while preserving `.tabura/prompt-injection.txt` overrides.

## Traceability

For publication metadata, associate this release with:

- release label: `v0.1.1`
- repository: `https://github.com/krystophny/tabura`
- exact source revision: tag target commit hash
