# Tabula Spec Index (Code-First)

Canonical behavior is code + tests. This file only points to source-of-truth locations.

## Contracts

1. Event schema + parsing
- `src/tabula/events.py`
- `tests/unit/test_events.py`
- `tests/unit/test_events_additional.py`

2. State reduction (`prompt`/`review`)
- `src/tabula/state.py`
- `tests/unit/test_state.py`
- `tests/bdd/test_mode_and_event_scenarios.py`

3. Canvas adapter behavior
- `src/tabula/canvas_adapter.py`
- `tests/unit/test_canvas_adapter_internal.py`
- `tests/bdd/test_canvas_adapter.py`

4. MCP server (`tabula-canvas`) stdio contract (framed + JSONL compatibility)
- `src/tabula/mcp_server.py`
- `tests/bdd/test_mcp_server.py`
- `tests/bdd/test_mcp_protocol_flows.py`
- `tests/integration/test_mcp_server_stdio.py`

5. Bootstrap protocol contract
- `src/tabula/protocol.py`
- `tests/unit/test_protocol_internal.py`
- `tests/bdd/test_protocol_bootstrap.py`

6. CLI surface (`canvas`, `schema`, `bootstrap`, `mcp-server`)
- `src/tabula/cli.py`
- `tests/bdd/test_cli_usage_modes.py`

7. Optional GUI canvas runtime
- `src/tabula/window.py`
- `tests/unit/test_window_mocked.py`
- `tests/gui/test_window_mode_switch.py`

8. Optional real interactive Codex loop (tmux + codex CLI)
- `tests/integration/test_codex_interactive_loop.py`
