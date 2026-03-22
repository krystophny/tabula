# Handoff Protocol v1 Overview

> **Legal notice:** Tabura is provided "as is" and "as available" without warranties, and to the maximum extent permitted by applicable law the authors/contributors accept no liability for damages, data loss, or misuse. You are solely responsible for backups, verification, and safe operation. See [`DISCLAIMER.md`](/DISCLAIMER.md).

## Goals

- Decouple producer/consumer from shared filesystem assumptions.
- Keep payload bytes out of LLM prompt/tool argument context.
- Provide a typed and versioned envelope for interoperability.

## Roles

- Producer: creates and serves handoff payloads.
- Consumer: imports payload and renders/uses it.

## Required producer tools

- `handoff.create`
- `handoff.peek`
- `handoff.consume`
- `handoff.revoke`
- `handoff.status`

## Kind notes

- `file` carries file metadata plus encoded content bytes.
- `mail` carries normalized email metadata and message body fields for downstream mail-aware consumers.

## Required envelope fields

- `spec_version` (example: `handoff.v1`)
- `handoff_id`
- `kind`
- `created_at` (RFC3339)
- `meta` (kind metadata)
- `payload` (kind payload)

## Policy model

- TTL (`expires_at` or `ttl_seconds` at create-time)
- Consume limit (`max_consumes`)
- Counters (`consumed_count`, `remaining_consumes`)
- Optional revocation (`revoked`)
