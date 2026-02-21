# Conformance Runner Notes

A runner should validate:

1. Envelope schema validation for positive examples.
2. Kind-specific schema validation for positive examples.
3. Negative examples must fail schema validation.
4. Producer behavior checks:
   - `consume` decrements/advances counters.
   - expired/revoked handoffs are rejected.

For message action profile v1, validate additionally:

1. Capability fixtures against `schemas/message-action-capabilities-v1.json`.
2. Request fixtures against `schemas/message-action-request-v1.json`.
3. Response fixtures against `schemas/message-action-response-v1.json`.
4. Negative defer request fixture (missing `until_at`) must fail validation.
5. Negative defer stub response fixture (invalid `stub_reason`) must fail validation.
