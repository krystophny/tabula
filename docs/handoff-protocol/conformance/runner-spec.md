# Conformance Runner Notes

> **Legal notice:** Tabura is provided "as is" and "as available" without warranties, and to the maximum extent permitted by applicable law the authors/contributors accept no liability for damages, data loss, or misuse. You are solely responsible for backups, verification, and safe operation. See [`DISCLAIMER.md`](/DISCLAIMER.md).

A runner should validate:

1. Envelope schema validation for positive examples.
2. Kind-specific schema validation for positive examples.
3. Negative examples must fail schema validation.
4. Producer behavior checks:
   - `consume` decrements/advances counters.
   - expired/revoked handoffs are rejected.
