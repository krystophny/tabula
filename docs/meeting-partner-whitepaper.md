# Retired Meeting-Partner Whitepaper

> **Legal notice:** Tabura is provided "as is" and "as available" without warranties, and to the maximum extent permitted by applicable law the authors/contributors accept no liability for damages, data loss, or misuse. You are solely responsible for backups, verification, and safe operation. See [`DISCLAIMER.md`](/DISCLAIMER.md).

This file previously described a `meeting-partner` product direction built on
top of Tabura's extension/plugin architecture.

That is now retired as the active roadmap.
The underlying product need may still survive inside Companion Mode, but not as
a private bundle product.

## Current Direction

- Meeting-notes and assistant-intervention work stays in the public
  `krystophny/tabura` repo
- Behavior should be implemented as normal modular core code
- No private meeting-partner bundle/repo is required
- Any retained interop surface should be a narrow local capability boundary,
  not a meeting-partner platform

## Public Tracking

- Architecture simplification: `#128`
- Directed-speech gate: `#129`
- Assistant response execution: `#130`
- Interaction policies: `#131`
- Room memory and entity timeline: `#132`

## Historical Note

Older release notes may still refer to "meeting-partner" as a prior design
exploration. Treat that as historical context, not current architecture.
