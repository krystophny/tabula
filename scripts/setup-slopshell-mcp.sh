#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

bash "$SCRIPT_DIR/setup-codex-mcp.sh"
bash "$SCRIPT_DIR/setup-claude-mcp.sh"
bash "$SCRIPT_DIR/setup-gemini-mcp.sh"
bash "$SCRIPT_DIR/setup-opencode-mcp.sh"

echo "Configured Codex, Claude, Gemini, and opencode to use the two external MCP servers: sloppy and helpy"
echo "slopshell itself remains UI/runtime only and is not registered as an agent MCP server"
