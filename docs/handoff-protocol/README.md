# Integrated Handoff Protocol Spec

> **Legal notice:** Tabura is provided "as is" and "as available" without warranties, and to the maximum extent permitted by applicable law the authors/contributors accept no liability for damages, data loss, or misuse. You are solely responsible for backups, verification, and safe operation. See [`DISCLAIMER.md`](/DISCLAIMER.md).

This directory carries the handoff protocol specification and conformance assets as part of the Tabura publication set.

Primary goals:
- keep the object-scoped UI paradigm as the top-level product framing
- keep transport/protocol contracts versioned and citable in the same public repository
- avoid split publication timing across multiple repos for core interoperability specs

Read in this order:
1. `spec/overview.md`
2. `spec/lifecycle.md`
3. `spec/mail.md`
4. `spec/security.md`
5. `security/threat-model.md`

Schemas:
- `schemas/envelope-v1.json`
- `schemas/kind-file-v1.json`
- `schemas/kind-mail-v1.json`
- `schemas/error-v1.json`

Conformance examples:
- `conformance/examples/*`
- `conformance/negative/*`
- `conformance/runner-spec.md`

Upstream snapshot note:
- `README.upstream.md` preserves the imported upstream overview text.

License:
- This integrated spec is distributed under repository MIT license (`LICENSE`).
