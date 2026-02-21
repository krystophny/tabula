# Threat Model (v1)

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
