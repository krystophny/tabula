#!/usr/bin/env bash
set -euo pipefail

MCP_URL="${1:-http://127.0.0.1:9420/mcp}"
CONFIG_PATH="${CODEX_CONFIG_PATH:-$HOME/.codex/config.toml}"

mkdir -p "$(dirname "$CONFIG_PATH")"
if [[ -f "$CONFIG_PATH" ]]; then
  cp "$CONFIG_PATH" "$CONFIG_PATH.bak.$(date +%Y%m%d%H%M%S)"
fi

python3 - "$CONFIG_PATH" "$MCP_URL" <<'PY'
from __future__ import annotations

import re
import sys
from pathlib import Path

path = Path(sys.argv[1])
mcp_url = sys.argv[2]

text = path.read_text(encoding="utf-8") if path.exists() else ""
block = "\n".join(
    [
        "# BEGIN TABULA MCP",
        "[mcp_servers.tabula]",
        f'url = "{mcp_url}"',
        "# END TABULA MCP",
        "",
    ]
)
pattern = re.compile(r"# BEGIN TABULA MCP\n.*?# END TABULA MCP\n?", re.S)
if pattern.search(text):
    updated = pattern.sub(block, text)
else:
    if text and not text.endswith("\n"):
        text += "\n"
    if text.strip():
        text += "\n"
    updated = text + block

path.write_text(updated, encoding="utf-8")
print(f"updated {path}")
print("server key: mcp_servers.tabula")
PY
