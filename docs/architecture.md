# Tabula Architecture (Go)

Tabula is a Go-first MCP canvas/runtime stack with a browser UI.

## Components

- `cmd/tabula/main.go`
  - CLI entrypoint and subcommand dispatch.
- `internal/mcp/server.go`
  - MCP JSON-RPC server (`initialize`, `tools/list`, `tools/call`).
- `internal/canvas/adapter.go`
  - Canvas/session/annotation state and tool implementations.
- `internal/serve/app.go`
  - Local MCP HTTP daemon (`/mcp`, `/ws/canvas`, `/files/*`).
- `internal/web/server.go`
  - Web UI backend (auth, host/session APIs, terminal/canvas websockets).
- `internal/ptyd/app.go`
  - PTY daemon to keep terminal sessions alive across web restarts.
- `internal/pty/*.go`
  - Local/PTYD PTY transport implementations.
- `internal/store/store.go`
  - SQLite persistence for web auth/hosts/session mappings.
- `internal/protocol/bootstrap.go`
  - Project bootstrap (`.tabula/*`, protocol files, gitignore wiring).

## Data Flow

1. Assistant calls MCP tools through `tabula mcp-server` (stdio) or `tabula serve` (HTTP MCP).
2. MCP calls resolve in `internal/canvas/adapter.go`.
3. Canvas events are broadcast over websocket to the browser canvas UI.
4. Web terminal traffic is handled via local PTY or PTYD transport.

## Runtime Modes

- CLI stdio MCP: `tabula mcp-server`
- Local MCP HTTP daemon: `tabula serve`
- Browser UI: `tabula web`
- Desktop canvas browser mode: `tabula canvas` (opens `/canvas`)
- PTY daemon: `tabula ptyd`
