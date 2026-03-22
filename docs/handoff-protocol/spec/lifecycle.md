# Lifecycle

> **Legal notice:** Tabura is provided "as is" and "as available" without warranties, and to the maximum extent permitted by applicable law the authors/contributors accept no liability for damages, data loss, or misuse. You are solely responsible for backups, verification, and safe operation. See [`DISCLAIMER.md`](/DISCLAIMER.md).

## handoff.create

Input:
- `kind`: producer kind such as `file` or `mail`
- `selector`: kind-specific source selection
  - `mail`: `account_id` plus `message_id` or `message_ids`
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
