# Slopshell Architecture

> **Legal notice:** Slopshell is provided "as is" and "as available" without warranties, and to the maximum extent permitted by applicable law the authors/contributors accept no liability for damages, data loss, or misuse. You are solely responsible for backups, verification, and safe operation. See [`DISCLAIMER.md`](/DISCLAIMER.md).

Slopshell is a Go monolithic UI/runtime with a split listener model:
- public web/UI listener
- private local runtime/control socket

There are exactly two external agent-facing MCP servers in the sloppy stack:

- `sloppy` = `sloptools mcp-server`
- `helpy` = `helpy mcp-stdio`

`slopshell` is not an external MCP server. The local Unix-socket routes
described here are private Slopshell runtime interfaces.

Runtime stack:
- `slopshell-web.service` runs the Go monolith (`slopshell server`)
- `helpy-mcp.service` runs the private helpy runtime daemon on `unix:$XDG_RUNTIME_DIR/sloppy/helpy.sock`
- `slopshell-codex-app-server.service` runs Codex app-server
- `slopshell-piper-tts.service` runs Piper TTS API on loopback
- `slopshell-llm.service` runs the Qwen3 0.6B local coordinator on loopback (`/v1/chat/completions`)

## Components

- `cmd/slopshell/main.go`
  - CLI entrypoint and subcommand dispatch.
- `internal/runtimecontrol/server.go`
  - Private runtime JSON-RPC methods and tool dispatch.
- `internal/canvas/adapter.go`
  - Canvas sessions, artifact state, and event log.
- `internal/serve/app.go`
  - Private runtime socket routes (including an intentionally undocumented
    control RPC path) and canvas websocket (`/ws/canvas`) mounted on the
    Unix-socket listener.
- `internal/web/server.go`
  - Browser APIs for chat sessions, canvas APIs, and chat/canvas websocket routes on the web listener.
- `internal/extensions/host.go`
  - Legacy manifest-driven compatibility runtime pending contraction,
    replacement, or removal. Loads only `*.extension.json` manifests.
- `internal/plugins/manager.go`
  - Legacy webhook compatibility runtime pending contraction, replacement, or
    removal. Loads only legacy plugin `*.json` manifests and ignores
    `*.extension.json` files so the two retained compatibility paths stay
    distinct.
- `internal/store/store.go`
  - SQLite persistence for workspaces, artifacts, items, actors, labels, chat, and auth.
- `internal/protocol/bootstrap.go`
  - Bootstrap behavior for project-local integration files.

## Runtime Modes

- `slopshell server`: monolithic runtime (web listener + private local runtime socket)
- `slopshell bootstrap`: project-local setup for Slopshell state and agent config helpers

## Local Sidecars

- Codex app-server remains a separate local service and is consumed over `ws://127.0.0.1:8787`.
- Piper TTS remains a separate local HTTP service on `http://127.0.0.1:8424`.
- Intent LLM remains a separate local HTTP service on `http://127.0.0.1:8081/v1/chat/completions`.
- Voxtype STT remains a separate local HTTP service on `http://127.0.0.1:8427/v1/audio/transcriptions`.
- Current Slopshell integration tracks voxtype branch `feature/single-daemon-openai-stt-api` from `https://github.com/peteonrails/voxtype`.
- Piper is intentionally not linked into the Go binary (`libpiper`) to avoid GPL-linked distribution coupling.

## UI Layout (Zen Canvas)

The browser UI is a full-viewport canvas with no visible chrome:

- **Tabula rasa**: blank white screen when no artifact is loaded.
- **Artifact mode**: document (text, image, PDF) fills the viewport.
- No toolbar, no prompt bar, no chat column. All interaction is invisible.
- **Edge panels** (hidden): top edge = workspace switcher, right edge = chat log / diagnostics. Revealed by hovering near screen edge (desktop) or swiping inward (mobile).

## Primary Data Flows

1. External agents call tools on `sloppy` or `helpy`; Slopshell itself uses its private local runtime/control socket where needed.
2. Tool dispatch in `internal/runtimecontrol/server.go` resolves into adapter operations.
3. Adapter updates session/artifact state in memory and emits events.
4. Browser consumes websocket events: responses stream into ephemeral overlay, artifacts update the canvas in place.

Chat hook flow:
1. Current code may route through legacy extension/plugin compatibility hooks.
2. New product behavior should stay in ordinary public core packages, not a new
   bundle ecosystem.
3. If any hook/API survives, it should be narrowed to explicit local
   capability-provider interop and deterministic compatibility needs.
4. Meeting-notes follow-up planning lives in public `sloppy-org/slopshell` issues only.

## Interaction Model

- **Tap/left-click** toggles voice recording. A red dot appears at the tap position.
- Pure VAD detects end-of-utterance and commits speech input.
- **Right-click** opens a floating text input at the cursor position.
- **Keyboard typing** (when nothing is focused) auto-activates text input.
- **Enter** sends the message; input is cleared.
- **Ctrl long-press** (300ms) starts push-to-talk; release stops and sends.
- **Escape** dismisses overlay/input. If nothing is open and an artifact is showing, clears to tabula rasa.
- On artifact: tap/right-click captures line context (`[Line N of "title"]`) prepended to the message.
- Responses stream as ephemeral overlays; click outside to dismiss. Document edits update the canvas in place with diff highlighting.

## Handoff Import Flow

1. Producer creates handoff payload (outside Slopshell).
2. Slopshell receives `canvas_import_handoff` with `handoff_id`.
3. Slopshell peeks/consumes producer handoff payload and renders artifact.

## Current Voice Runtime and Live Sessions

Slopshell now exposes one `Live` entry point with two policy variants:

- `Dialogue`
- `Meeting`

Both share the same browser-side live session owner, hotword pipeline, and
voice capture path. The built-in hotword target is `Computer`.

Wake-word detection runs entirely in the browser using ONNX Runtime Web:
- `melspectrogram.onnx` extracts mel features from raw audio.
- `embedding_model.onnx` produces frame-level embeddings.
- `keyword.onnx` is the active keyword classifier (16-frame input, ~1.28s detection latency).

The shared feature extractors live in `internal/web/static/vendor/openwakeword/`.
The active keyword model is installed into the runtime data directory and served as `keyword.onnx`.

Audio pipeline in `hotword.js`:
- Mic audio is downsampled to 16 kHz mono via a ScriptProcessorNode.
- Each audio frame is written to a 2-second ring buffer (32,000 samples) for pre-roll capture.
- Mel and embedding stages feed into the keyword classifier per frame.
- On wake-word detection, the app begins voice recording immediately (no intermediate listen window).

State transitions:
- **Quiet**: meeting live session is active and listening for context.
- **Paused** (black border + pause bars): a live session is active and waiting for `Computer`.
- **Recording** (red border + red dot): wake word detected or user tapped, capturing speech.
- **Listening** (blue border + pulse): dialogue follow-up window after TTS response.
- Follow-up timeout returns to **Paused** and restarts hotword monitoring.

Control surfaces:
- The web runtime uses a single floating `#slopshell-circle` for tool selection, Dialogue/Meeting activation, and the Silent toggle.
- The top edge panel is reduced to workspace navigation and runtime summary only.
- Configuration-heavy surfaces such as hotword/model/voice management live under `/manage` instead of the canvas shell.

Utterance filtering (server-side in `internal/stt/transcribe.go`):
- Whisper hallucination blocklist (13 phrases).
- Noise rejection: filler-only transcripts (<3 words), TV/radio background patterns.
- Minimum audio buffer size (1024 bytes).

## STT Sidecar

- `slopshell-stt.service` runs voxtype on loopback (`http://127.0.0.1:8427/v1/audio/transcriptions`).
- For source builds, use voxtype branch `feature/single-daemon-openai-stt-api` until this lands in an upstream release.
- Audio flows: browser WebSocket -> RAM buffer -> HTTP POST to sidecar -> transcript text returned.
- No audio is persisted to disk or database. See `docs/meeting-notes-privacy.md`.

## Trust and Access Boundaries

- Slopshell does not require direct credentials to producer systems.
- Producer endpoint authority remains outside Slopshell.
- Slopshell stores local auth/session state in SQLite under web data dir.
- Private runtime routes are not mounted on the web listener and stay on the private Unix socket.
- Slopshell is UI/runtime only; backend/domain authority lives in `sloptools` (`sloppy`) and `helpy`.

## Modular Core Direction

Slopshell's active direction is a single public repo with ordinary modular
packages under `internal/`. Product behavior should live in public core code,
not a private repo and not an extension/plugin bundle system.

Auth/session, media transport, queueing, persistence, privacy invariants, and
meeting-notes behavior stay in core. The legacy `internal/extensions` and
`internal/plugins` packages should be treated as transitional compatibility or
interop code rather than an expanding product surface.

If a compatibility surface remains during cleanup, it should be justified as a
small local capability boundary for deterministic external integrations such as
Helpy, not as a general SDK.
