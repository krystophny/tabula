# Tabura Plugin Boundaries and System

Tabura supports server-side plugins, while keeping runtime safety guarantees in
core.

## Core Runtime (Non-Plugin)

These concerns stay in this repository and are not delegated to plugins:

- Auth/session/cookie enforcement and API access control.
- Chat turn queueing, cancellation, websocket transport, and persistence.
- STT/TTS transport primitives and media validation.
- Privacy invariants for meeting notes (RAM-only audio, no audio persistence).
- Canvas/file safety boundaries and path constraints.

## Plugin Space

Plugins own product-specific decision logic and capability modules.

Primary target domain: `meeting-partner` for always-listen policy, directed
speech detection, and intelligent response strategy from transcript/event
context.

## Loading Model

- Plugin manifests are JSON files in `TABURA_PLUGINS_DIR`.
- Default directory: `<data-dir>/plugins` (for example `~/.tabura-web/plugins`).
- Set `TABURA_PLUGINS_DIR=off` to disable loading.
- Only enabled plugins are loaded (`"enabled": true`).

Runtime introspection:

- `GET /api/runtime` returns:
  - `plugins_dir`
  - `plugins_loaded`
- `GET /api/plugins` returns loaded plugin inventory.

## Manifest Format

```json
{
  "id": "always-on-partner",
  "kind": "webhook",
  "endpoint": "http://127.0.0.1:9901/hooks/always-on",
  "hooks": [
    "chat.pre_user_message",
    "chat.pre_assistant_prompt",
    "chat.post_assistant_response"
  ],
  "timeout_ms": 1200,
  "enabled": true,
  "secret_env": "TABURA_PLUGIN_SECRET"
}
```

Notes:

- `kind` currently supports `webhook` only.
- `timeout_ms` is capped at `30000`.
- If `secret_env` is set and present in environment, Tabura sends:
  - `Authorization: Bearer <value>`

## Hook Contract

Tabura sends a JSON POST request to plugin endpoints:

```json
{
  "hook": "chat.pre_user_message",
  "session_id": "chat-session-id",
  "project_key": "project-key",
  "output_mode": "voice",
  "text": "raw text",
  "metadata": {
    "local_only": false
  }
}
```

Plugin response:

```json
{
  "text": "possibly rewritten text",
  "blocked": false,
  "reason": ""
}
```

Behavior:

- `text`: optional rewrite (if present, Tabura uses it).
- `blocked=true`: request/turn is rejected with `reason`.
- Plugin HTTP failures are non-fatal: Tabura logs and continues.

## Built-in Hook Points

- `chat.pre_user_message`
  - Runs before storing user text and before command detection.
- `chat.pre_assistant_prompt`
  - Runs before sending prompt to app-server.
- `chat.post_assistant_response`
  - Runs before assistant response persistence/broadcast.

## Repository Split

- `tabura` keeps runtime substrate and guarantees.
- `tabura-plugins` (private) owns premium/product plugin implementations.
