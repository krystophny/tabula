# Retired Bundle Platform Whitepaper

> **Legal notice:** Tabura is provided "as is" and "as available" without warranties, and to the maximum extent permitted by applicable law the authors/contributors accept no liability for damages, data loss, or misuse. You are solely responsible for backups, verification, and safe operation. See [`DISCLAIMER.md`](/DISCLAIMER.md).

This file previously documented a local-first extension platform for Tabura.

That general bundle-platform direction is no longer the active product
direction. The useful residue is a much smaller question: what narrow
integration boundary, if any, should survive for local capability providers.

## Current Direction

- One public repo for product behavior: `krystophny/tabura`
- Modular internal packages instead of extension bundles
- No private premium bundle repo as part of the intended architecture
- Optional external capabilities should integrate through explicit local
  protocols, not a broad extension SDK

## Practical Implications

- UI work stays in core web code
- Meeting-notes logic stays in core meeting-notes code
- Planning and tracking stay in public GitHub issues
- Existing extension/plugin runtime code should be treated as legacy
  compatibility code pending contraction, replacement, or removal
- If any compatibility API survives, it should become smaller, more explicit,
  and easier to test than the old bundle model

## Acceptable Surviving Surface

The bar for keeping an old extension-era API is:

- it directly supports a current public-core workflow
- it cleanly bridges to a local capability provider such as Helpy
- it has a deterministic contract and a bounded trust model
- it is simpler than replacing it immediately

Otherwise, remove it.

## Public Tracking

- `#128` Companion Mode umbrella and modular-core direction
- `#139` Radical cleanup with explicit justification for any retained
  compatibility surface

## Historical Note

Release notes and older commits may still mention extension-platform work. That
material is historical and should not be used to steer new feature design.
