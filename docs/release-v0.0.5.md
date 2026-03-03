# Release v0.0.5

> **Legal notice:** Tabura is provided "as is" and "as available" without warranties, and to the maximum extent permitted by applicable law the authors/contributors accept no liability for damages, data loss, or misuse. You are solely responsible for backups, verification, and safe operation. See [`DISCLAIMER.md`](/DISCLAIMER.md).

> Historical note: This file documents a past release. For the current runtime stack, use README.md and docs/architecture.md.

## Scope

`v0.0.5` introduces Push To Prompt voice capture backed by VoxType MCP and removes the Helpy STT provider path from Tabura.

## Highlights

- Added `tabura voxtype-mcp` command and local VoxType MCP server (`/mcp`, `/health`).
- Added user `systemd` unit `tabura-voxtype-mcp.service` for always-on local bridge mode.
- Added streaming Push To Prompt web API: `POST /api/stt/push-to-prompt` with `start`, `append`, `stop`, `cancel`.
- Updated mail voice drafting flow to stream audio chunks to Push To Prompt for faster transcription turnaround.
- VoxType MCP bridge now prefers daemon-backed capture to reuse already-running `voxtype.service`.
- Kept `POST /api/mail/stt` as compatibility entrypoint, now implemented via VoxType MCP internally.
- Standardized terminology in docs/UI as **Push To Prompt**.
- Bumped runtime surface versions to `0.0.5`.

## Interface Stability Notes

- Helpy STT integration was removed from Tabura in this release.
- Voice/STT behavior now depends on a loopback VoxType MCP endpoint.

## Traceability

For publication metadata, associate this release with:

- release label: `v0.0.5`
- repository: `https://github.com/krystophny/tabura`
- exact source revision: tag target commit hash
