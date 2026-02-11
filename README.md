# tabula

Minimal Codex-centric canvas adapter.

Tabula is no longer a workflow orchestrator. Codex is the master process.
Tabula provides:

- an MCP server (`tabula-canvas`) for canvas tools
- strict event schema/validation utilities
- optional local canvas window runtime
- project bootstrap (`AGENTS.md` + MCP snippet + artifact folders)

## Install

```bash
python -m pip install -e .[test]
python -m pip install -e .[gui]   # optional for local canvas window
```

## Core commands

```bash
tabula bootstrap --project-dir .
tabula mcp-server --project-dir . --headless --no-canvas
tabula canvas --events .tabula/canvas-events.jsonl
tabula check-events --events .tabula/canvas-events.jsonl
tabula schema
```

## Codex MCP integration

`tabula bootstrap` writes `.tabula/codex-mcp.toml` with a snippet like:

```toml
[mcp_servers.tabula-canvas]
command = "tabula"
args = ["mcp-server", "--project-dir", "/abs/path/to/project"]
```

Merge that snippet into `~/.codex/config.toml`.

## MCP tools exposed

- `canvas_activate`
- `canvas_render_text`
- `canvas_render_image`
- `canvas_render_pdf`
- `canvas_clear`
- `canvas_status`

All render tools emit strict JSONL events into `.tabula/canvas-events.jsonl`.

## Tests

```bash
PYTHONPATH=src python -m pytest
```

Optional real interactive Codex E2E (tmux terminal session, no `codex exec`):

```bash
TABULA_RUN_REAL_CODEX_INTERACTIVE=1 PYTHONPATH=src python -m pytest tests/integration/test_codex_interactive_loop.py -q
```
