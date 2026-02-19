#!/usr/bin/env bash
set -euo pipefail

MCP_URL="${1:-http://127.0.0.1:9420/mcp}"
SETTINGS_PATH="${CLAUDE_SETTINGS_PATH:-$HOME/.claude/settings.json}"

mkdir -p "$(dirname "$SETTINGS_PATH")"
if [[ -f "$SETTINGS_PATH" ]]; then
  cp "$SETTINGS_PATH" "$SETTINGS_PATH.bak.$(date +%Y%m%d%H%M%S)"
fi

python3 - "$SETTINGS_PATH" "$MCP_URL" <<'PY'
from __future__ import annotations

import json
import sys
from pathlib import Path

path = Path(sys.argv[1])
mcp_url = sys.argv[2]

if path.exists() and path.read_text(encoding="utf-8").strip():
    data = json.loads(path.read_text(encoding="utf-8"))
else:
    data = {}

if not isinstance(data, dict):
    raise SystemExit(f"invalid JSON object in {path}")

servers = data.get("mcpServers")
if not isinstance(servers, dict):
    servers = {}
data["mcpServers"] = servers
servers.pop("tabula-broker", None)
servers["tabula"] = {"url": mcp_url}

path.write_text(json.dumps(data, indent=2, sort_keys=True) + "\n", encoding="utf-8")
print(f"updated {path}")
print("server key: mcpServers.tabula")
PY
