# Release v0.1.5

> **Legal notice:** Tabura is provided "as is" and "as available" without warranties, and to the maximum extent permitted by applicable law the authors/contributors accept no liability for damages, data loss, or misuse. You are solely responsible for backups, verification, and safe operation. See [`DISCLAIMER.md`](/DISCLAIMER.md).

## Scope

`v0.1.5` finalizes the STT runtime transition to voxtype sidecar mode, hardens tap-to-stop behavior during transcript send, and aligns docs with the active runtime stack.

## Highlights

### STT Runtime and Packaging

- Removed remaining `whisper.cpp` setup path references from Tabura runtime/docs.
- Renamed installer helper from `setup-whisper-stt.sh` to `setup-voxtype-stt.sh`.
- Standardized STT sidecar references to OpenAI-compatible endpoint `/v1/audio/transcriptions`.

### Voice Stop/Send Reliability

- Hardened voice lifecycle handling around stop gestures while transcript submit is pending.
- Added deterministic abort-aware submit guard so stop requests win over in-flight transcript sends.
- Verified with full end-to-end Playwright suite.

### Documentation Alignment

- Updated runtime docs to include explicit `tabura-stt.service` voxtype sidecar usage.
- Added temporary voxtype source pin for Tabura integrations:
  - repo: `https://github.com/peteonrails/voxtype`
  - branch: `feature/single-daemon-openai-stt-api`

### Metadata and Versioning

- Bumped runtime metadata to `v0.1.5` in release-visible version fields.

## Traceability

For publication metadata, associate this release with:

- release label: `v0.1.5`
- repository: `https://github.com/krystophny/tabura`
- exact source revision: tag target commit hash
