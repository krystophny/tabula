# Mail Handoff Kind

> **Legal notice:** Tabura is provided "as is" and "as available" without warranties, and to the maximum extent permitted by applicable law the authors/contributors accept no liability for damages, data loss, or misuse. You are solely responsible for backups, verification, and safe operation. See [`DISCLAIMER.md`](/DISCLAIMER.md).

## Producer selector

Create a mail handoff with:
- `kind: mail`
- `selector.account_id`
- `selector.message_id` or `selector.message_ids`

Tabura resolves those message IDs through the configured mail provider and builds one envelope from normalized message metadata.

## Envelope shape

`meta` fields:
- `account.id`
- `account.name`
- `account.provider`
- `account.sphere`
- `message_count`
- `message_ids`
- `subjects`
- `senders`
- `recipients`
- `dates`
- `internet_message_ids`
- `thread_ids`
- `attachment_count`
- `contains_rich_content`

`payload.messages[]` fields:
- `message_id`
- `thread_id`
- `internet_message_id`
- `subject`
- `sender`
- `recipients`
- `date`
- `snippet`
- `labels`
- `is_read`
- `is_flagged`
- `body_text` when present
- `body_html` when present
- `attachments[]` with `id`, `filename`, `mime_type`, `size`, `is_inline`

## Downstream usage

Intended consumers can:
- inspect `meta` during routing without pulling full message bodies into unrelated flows
- consume `payload.messages` to render mail summaries, start triage flows, or bridge into another mail-aware MCP service
- rely on the generic lifecycle counters and revocation state exposed by `handoff.peek`, `handoff.consume`, `handoff.revoke`, and `handoff.status`
