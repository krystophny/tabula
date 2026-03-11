# Release v0.2.0

> **Legal notice:** Tabura is provided "as is" and "as available" without warranties, and to the maximum extent permitted by applicable law the authors/contributors accept no liability for damages, data loss, or misuse. You are solely responsible for backups, verification, and safe operation. See [`DISCLAIMER.md`](/DISCLAIMER.md).

## Scope

`v0.2.0` publishes the current Daily Workspace runtime, the live `Dialogue` and `Meeting` interaction model, and the provider-routed execution stack that now defines shipped Tabura behavior.

## Highlights

### Workspace and Interaction Model

- Daily Workspace is the default active workspace with persistent date-based context under the data directory hierarchy.
- Live interaction is organized around two explicit modes only: `Dialogue` for direct turn-taking and `Meeting` for ambient capture plus selective intervention.
- Workspace-aware canvas and artifact flows now define the primary UX instead of legacy project/session framing.

### Runtime and Provider Routing

- The public runtime remains a single `tabura server` process with loopback-only MCP served alongside the web UI.
- Local intent routing remains available through `tabura-llm.service`, while Spark stays the default app-server model and optional Cerebras and Gemini providers extend execution for broader tasks.
- Runtime-facing version surfaces are aligned across the CLI binary, web runtime, MCP server, and app-server client/session handshakes.

### Documentation and Release Surfaces

- README and spec-index release pointers now reference `v0.2.0`.
- Release verification commands are aligned with the current maintenance policy: surface sync check, Go tests, and Playwright.
- Citation and archival metadata now publish `v0.2.0`.

## Verification Scope

This release is intended to be verified against:

- `./scripts/sync-surface.sh --check`
- `go test ./...`
- `./scripts/playwright.sh`
- `scripts/check-version-consistency.sh`

## Traceability

For publication metadata, associate this release with:

- release label: `v0.2.0`
- repository: `https://github.com/krystophny/tabura`
- exact source revision: tag target commit hash
