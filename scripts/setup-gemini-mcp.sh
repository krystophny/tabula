#!/usr/bin/env bash
# Register sloppy and helpy as stdio MCP servers in Gemini CLI.
set -euo pipefail

SLOPTOOLS_BIN="${SLOPSHELL_SLOPTOOLS_BIN:-$HOME/.local/bin/sloptools}"
HELPY_BIN="${SLOPSHELL_HELPY_BIN:-$HOME/.local/bin/helpy}"
SLOPPY_PROJECT_DIR="${SLOPSHELL_SLOPPY_PROJECT_DIR:-$HOME}"
SLOPPY_DATA_DIR="${SLOPSHELL_SLOPPY_DATA_DIR:-$HOME/.local/share/sloppy}"
VAULT_CONFIG="${SLOPTOOLS_VAULT_CONFIG:-$HOME/.config/sloptools/vaults.toml}"

if ! command -v gemini >/dev/null 2>&1; then
  echo "gemini CLI not found; skipping" >&2
  exit 0
fi

gemini mcp remove slopshell >/dev/null 2>&1 || true
gemini mcp remove sloptools >/dev/null 2>&1 || true

gemini mcp remove sloppy >/dev/null 2>&1 || true
gemini mcp add sloppy "$SLOPTOOLS_BIN" mcp-server   --stdio --vault-config "$VAULT_CONFIG" --project-dir "$SLOPPY_PROJECT_DIR" --data-dir "$SLOPPY_DATA_DIR"

gemini mcp remove helpy >/dev/null 2>&1 || true
gemini mcp add helpy "$HELPY_BIN" mcp-stdio

echo "registered sloppy and helpy with gemini"
