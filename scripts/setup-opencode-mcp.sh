#!/usr/bin/env bash
# Register sloppy and helpy as local MCP servers in opencode and remove stale
# slopshell/sloptools entries.
set -euo pipefail

CONFIG_PATH="${OPENCODE_CONFIG:-${HOME}/.config/opencode/opencode.json}"
SLOPTOOLS_BIN="${SLOPSHELL_SLOPTOOLS_BIN:-$HOME/.local/bin/sloptools}"
HELPY_BIN="${SLOPSHELL_HELPY_BIN:-$HOME/.local/bin/helpy}"
SLOPPY_PROJECT_DIR="${SLOPSHELL_SLOPPY_PROJECT_DIR:-$HOME}"
SLOPPY_DATA_DIR="${SLOPSHELL_SLOPPY_DATA_DIR:-$HOME/.local/share/sloppy}"
VAULT_CONFIG="${SLOPTOOLS_VAULT_CONFIG:-$HOME/.config/sloptools/vaults.toml}"

mkdir -p "$(dirname "$CONFIG_PATH")" "$SLOPPY_DATA_DIR"

if [[ ! -f "$CONFIG_PATH" ]]; then
  echo '{"$schema":"https://opencode.ai/config.json"}' >"$CONFIG_PATH"
fi

python3 - "$CONFIG_PATH" "$SLOPTOOLS_BIN" "$HELPY_BIN" "$SLOPPY_PROJECT_DIR" "$SLOPPY_DATA_DIR" "$VAULT_CONFIG" <<'PY'
import json
import sys

config_path, sloptools_bin, helpy_bin, project_dir, data_dir, vault_config = sys.argv[1:7]

with open(config_path) as f:
    config = json.load(f)

if not isinstance(config, dict):
    raise SystemExit("opencode config root must be a JSON object")

mcp = config.get("mcp")
if not isinstance(mcp, dict):
    mcp = {}
config["mcp"] = mcp

mcp["sloppy"] = {
    "type": "local",
    "command": [
        sloptools_bin,
        "mcp-server",
        "--stdio",
        "--vault-config", vault_config,
        "--project-dir", project_dir,
        "--data-dir", data_dir,
    ],
    "enabled": True,
}
mcp["helpy"] = {
    "type": "local",
    "command": [helpy_bin, "mcp-stdio"],
    "enabled": True,
}

mcp.pop("sloptools", None)
mcp.pop("slopshell", None)

with open(config_path, "w") as f:
    json.dump(config, f, indent=2)
    f.write("\n")
PY

echo "updated $CONFIG_PATH"
echo "server keys: mcp.sloppy, mcp.helpy"
