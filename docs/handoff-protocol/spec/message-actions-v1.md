# Message Actions Profile v1

This profile defines deterministic action payloads for UI-triggered message triage.
It is intentionally non-LLM and transport-agnostic.

## Capability payload

Required boolean fields:

- `supports_open`
- `supports_archive`
- `supports_delete_to_trash`
- `supports_native_defer`

## Action request payload

Required:

- `action`: `open` | `archive` | `delete` | `defer`
- `message_id`

Conditional required:

- `until_at` when `action=defer`, RFC3339.

## Action response payload

Required:

- `action`
- `message_id`
- `status`: `ok` | `stub_not_supported` | `error`
- `effective_provider_mode`: `native` | `stub`

Recommended:

- `provider`
- `deferred_until_at` for successful defer
- `error_code`, `error_message` for status `error`
- `stub_reason` for status `stub_not_supported`

## Provider semantics

### Gmail defer

- Use provider-native defer/snooze.
- Return `effective_provider_mode=native`.

### IMAP defer (v1)

- Return structured stub response.
- Do not mutate inbox state in v1.
- Return `effective_provider_mode=stub` and `status=stub_not_supported`.
- Return `stub_reason=DEFER_NOT_SUPPORTED_FOR_PROVIDER`.
