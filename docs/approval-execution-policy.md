# Approval and Execution Policy

Tabura has one execution-policy model. `plan`, `review`, and `yolo` are not separate product universes; they only change how risky work is gated.

## Policy Table

| Policy | Read/search | Canvas changes | File edits | External actions |
| --- | --- | --- | --- | --- |
| `default` | Allowed without extra approval | Allowed in the normal artifact flow | `approvalPolicy=on-request`; destructive shell commands also require explicit next-message `confirm` | `approvalPolicy=on-request` |
| `plan` / `review` | Allowed, but multi-step work should be proposed before execution | Allowed in the normal artifact flow | `approvalPolicy=unlessTrusted`; destructive shell commands still require explicit next-message `confirm` | `approvalPolicy=unlessTrusted` |
| `yolo` / `autonomous` | Allowed | Allowed in the normal artifact flow | `approvalPolicy=never`; destructive-shell confirmation is bypassed | `approvalPolicy=never` |

The runtime toggle is labeled `Auto` in the top edge controls. That label means `autonomous` execution policy, not a separate interaction mode.

## Canonical Approval Flow

- Approval requests render as temporary canvas artifacts in the same canvas/artifact flow as other assistant output.
- The same approval request is mirrored in chat so the decision is reachable from either surface.
- Approval decisions are sent as `approval_response` events and resolved in both surfaces together.

## Source of Truth

- Session approval mapping: `internal/web/chat_approval.go`
- Session execution policy mapping: `internal/web/execution_policy.go`
- Approval policy normalization: `internal/appserver/approval.go`
- Destructive-command confirmation guard: `internal/web/action_policy.go`
- Canvas rendering path for approval requests: `internal/web/static/app-chat-transport.js` and `internal/web/static/canvas.js`
