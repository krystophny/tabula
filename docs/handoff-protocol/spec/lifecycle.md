# Lifecycle

## handoff.create

Input:
- `kind`: `file` | `mail_headers`
- `selector`: kind-specific source selection
- `policy` (optional): `ttl_seconds`, `expires_at`, `max_consumes`

Output:
- `spec_version`, `handoff_id`, `kind`, `meta`, `created_at`, `policy_summary`

## handoff.peek

Input:
- `handoff_id`

Output:
- Same as create metadata, no payload.

## handoff.consume

Input:
- `handoff_id`

Output:
- `spec_version`, `handoff_id`, `kind`, `created_at`, `meta`, `payload`, `policy`

## handoff.revoke

Input:
- `handoff_id`

Output:
- Revocation acknowledgement + policy summary.

## handoff.status

Input:
- `handoff_id`

Output:
- Metadata + policy counters + revocation state.

## Message Actions (profile)

This protocol family also defines payload contracts for deterministic message actions.
These are transport-agnostic and can be carried by MCP tools or HTTP APIs.

Core request shape:
- `action`: `open` | `archive` | `delete` | `defer`
- `message_id`
- `until_at` (required when `action=defer`)

Core response shape:
- `action`
- `message_id`
- `status`: `ok` | `stub_not_supported` | `error`
- `effective_provider_mode`: `native` | `stub`

For IMAP defer v1, standardized stub behavior is:
- `action=defer`
- `status=stub_not_supported`
- `effective_provider_mode=stub`
- `stub_reason=DEFER_NOT_SUPPORTED_FOR_PROVIDER`
