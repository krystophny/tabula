# Helpy Interop Direction

> **Legal notice:** Tabura is provided "as is" and "as available" without warranties, and to the maximum extent permitted by applicable law the authors/contributors accept no liability for damages, data loss, or misuse. You are solely responsible for backups, verification, and safe operation. See [`DISCLAIMER.md`](/DISCLAIMER.md).

This document used to track functionality that was intended to move into a
private Helpy repo.

That recovery-plan framing is no longer the active direction. The relevant
future question is how Tabura should interoperate with Helpy without reviving a
private extension ecosystem.

## Current Direction

- Default product behavior should stay public in `krystophny/tabura`
- No private recovery repo is required for the meeting-notes or assistant
  roadmap
- Helpy may still be a useful optional local capability provider
- Any future external-service integrations should be optional sidecars, MCP
  servers, or clearly documented public dependencies

## Candidate Helpy Responsibilities

If Tabura uses Helpy in the future, the likely responsibilities are:

- deterministic office-workflow actions
- email listing, reading, and message actions
- calendar and ICS access
- sheet inspection
- handoff production or consumption across local tools

Tabura should not depend on Helpy for:

- its default speech-to-text path
- its core privacy invariants
- its primary runtime state machine
- ownership of Companion Mode behavior

## API Guidance

If legacy extension/plugin APIs are kept for Helpy-related interop, revise them
to be:

- smaller in scope
- capability-oriented rather than bundle-oriented
- loopback-only or otherwise explicitly local
- deterministic and testable
- replaceable by MCP or dedicated local HTTP contracts later

If those properties are not met, remove the API instead of preserving it for
sentimental compatibility.

## Historical Note

Older commits and scripts may reference a private Helpy recovery path. Treat
those references as retired planning only.
