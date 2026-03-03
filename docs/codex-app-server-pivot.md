# Codex + TTS Integration (Current)

> **Legal notice:** Tabura is provided "as is" and "as available" without warranties, and to the maximum extent permitted by applicable law the authors/contributors accept no liability for damages, data loss, or misuse. You are solely responsible for backups, verification, and safe operation. See [`DISCLAIMER.md`](/DISCLAIMER.md).

This document defines the current local integration model used by Tabura.

## Canonical Local Stack

1. `tabura-web.service` runs `tabura server` (Go monolith).
2. `tabura-codex-app-server.service` runs `codex app-server --listen ws://127.0.0.1:8787`.
3. `tabura-piper-tts.service` runs Piper TTS on `http://127.0.0.1:8424`.

Tabura web depends on both local sidecars.

## Why Codex App Server Is Kept

- Tabura uses Codex app-server for Codex-like thread/turn/session behavior.
- Integration uses local WebSocket JSON-RPC.
- This preserves the same runtime model as Codex tooling while keeping Tabura UI/runtime control.

## Why Piper Is Kept as HTTP Sidecar

- Piper is consumed through a local OpenAI-compatible endpoint (`/v1/audio/speech`).
- Keeping Piper as a sidecar keeps deployment simple and avoids direct `libpiper` GPL-linking concerns in the Go binary.
- Loopback transport overhead is small relative to synthesis time and is acceptable for current UX.

## Data Paths

1. Browser WS -> Tabura web (`/ws/chat/{session_id}`).
2. Tabura web -> Codex app-server (`ws://127.0.0.1:8787`) for assistant turns.
3. Tabura web -> Piper (`http://127.0.0.1:8424/v1/audio/speech`) for streamed speech audio.

## Operational Commands

Status:

```bash
systemctl --user status tabura-web.service tabura-codex-app-server.service tabura-piper-tts.service --no-pager -n 40
```

Restart:

```bash
systemctl --user restart tabura-codex-app-server.service tabura-piper-tts.service tabura-web.service
```

## Historical Note

Legacy sidecars such as `tabura-mcp.service`, `tabura-voxtype-mcp.service`, and `tabura-ptyd.service` are retired and not part of the current runtime model.
