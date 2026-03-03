# Threat Model (v1)

> **Legal notice:** Tabura is provided "as is" and "as available" without warranties, and to the maximum extent permitted by applicable law the authors/contributors accept no liability for damages, data loss, or misuse. You are solely responsible for backups, verification, and safe operation. See [`DISCLAIMER.md`](/DISCLAIMER.md).

## Threats

- Replay of handoff IDs
- Unauthorized consume by wrong consumer
- Tampered payload in transit/storage
- Overlong lifetime leading to stale credential abuse

## Mitigations

- Single-use default and strict consume counters
- Authn/authz on producer MCP endpoint
- Integrity checks (`sha256`, `size_bytes`) for file payloads
- Short TTL defaults and explicit revocation support
