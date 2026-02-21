# Handoff Protocol v1 Overview

## Goals

- Decouple producer/consumer from shared filesystem assumptions.
- Keep payload bytes out of LLM prompt/tool argument context.
- Provide a typed and versioned envelope for interoperability.
- Provide deterministic, non-LLM action payload contracts for message triage UX.

## Roles

- Producer: creates and serves handoff payloads.
- Consumer: imports payload and renders/uses it.

## Required producer tools

- `handoff.create`
- `handoff.peek`
- `handoff.consume`
- `handoff.revoke`
- `handoff.status`

## Message action profile (v1)

This repo also defines v1 payload contracts for deterministic message actions:

- capabilities: `supports_open`, `supports_archive`, `supports_delete_to_trash`, `supports_native_defer`
- actions: `open`, `archive`, `delete`, `defer`
- defer request: requires `until_at` (RFC3339)
- defer response: includes `status` and `effective_provider_mode` (`native` | `stub`)

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
