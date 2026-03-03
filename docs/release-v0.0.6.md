# Release v0.0.6

> **Legal notice:** Tabura is provided "as is" and "as available" without warranties, and to the maximum extent permitted by applicable law the authors/contributors accept no liability for damages, data loss, or misuse. You are solely responsible for backups, verification, and safe operation. See [`DISCLAIMER.md`](/DISCLAIMER.md).

> Historical note: This file documents a past release. For the current runtime stack, use README.md and docs/architecture.md.

## Scope

`v0.0.6` adds multi-model chat architecture with `gpt-5.3-codex-spark` as the fast default, canvas-aware system prompts, and fixes iOS mic battery drain.

## Highlights

### Multi-Model Chat Architecture
- Set `gpt-5.3-codex-spark` as default model for outer chat loop (1000+ tok/s on Cerebras)
- `--model` CLI flag, `TABURA_APP_SERVER_MODEL` env var, and hardcoded default fallback
- Per-turn model override via `TurnModel` in `PromptRequest` for delegating to heavier models
- `available_models` list exposed in `/api/runtime`

### Canvas-Aware System Prompt
- Rewrote `buildPromptFromHistory` with system instruction block: Tabura identity, canvas action markers, guidelines
- `:::canvas_show{title="..." kind="..."}...:::` markers parsed from assistant responses and pushed to canvas
- Active canvas artifact context injected into prompts

### iOS Mic Battery Fix
- Mic permission prompt still fires on page load for instant recording
- Stream immediately released after permission grant (no persistent hardware hold)
- `releaseMicStream()` called after each voice capture completes

### VoxType MCP Simplification
- Removed daemon capture mode; browser-buffered audio is the sole capture path
- Simplified VoxType MCP server tool handlers

### Rename and Cleanup
- Complete `tabula` to `tabura` rename across all files, systemd units, scripts
- WebSocket STT migration from HTTP to streaming protocol

## Migration

- The `capture_mode` parameter on `push_to_prompt_start` is no longer accepted
- The `voxtype.service` system daemon is no longer required
- Set `TABURA_APP_SERVER_MODEL` in systemd unit or use `--model` flag (defaults to `gpt-5.3-codex-spark`)

## Traceability

For publication metadata, associate this release with:

- release label: `v0.0.6`
- repository: `https://github.com/krystophny/tabura`
- exact source revision: tag target commit hash
