from __future__ import annotations

import json
import os
import subprocess
from pathlib import Path


def _scripts_dir() -> Path:
    return Path(__file__).resolve().parents[2] / "scripts"


def test_setup_codex_mcp_is_idempotent(tmp_path: Path) -> None:
    config = tmp_path / "config.toml"
    config.write_text('[profiles.default]\nmodel = "gpt-5"\n', encoding="utf-8")
    script = _scripts_dir() / "setup-codex-mcp.sh"

    env = dict(os.environ)
    env["CODEX_CONFIG_PATH"] = str(config)

    subprocess.run([str(script), "http://127.0.0.1:9420/mcp"], check=True, env=env)
    subprocess.run([str(script), "http://127.0.0.1:9420/mcp"], check=True, env=env)

    text = config.read_text(encoding="utf-8")
    assert "[profiles.default]" in text
    assert text.count("# BEGIN TABULA MCP") == 1
    assert "[mcp_servers.tabula]" in text
    assert 'url = "http://127.0.0.1:9420/mcp"' in text


def test_setup_claude_mcp_preserves_existing_fields(tmp_path: Path) -> None:
    settings = tmp_path / "settings.json"
    settings.write_text(
        json.dumps({"theme": "light", "mcpServers": {"tabula-broker": {"url": "http://127.0.0.1:9420/mcp"}}}),
        encoding="utf-8",
    )
    script = _scripts_dir() / "setup-claude-mcp.sh"

    env = dict(os.environ)
    env["CLAUDE_SETTINGS_PATH"] = str(settings)

    subprocess.run([str(script), "http://127.0.0.1:9420/mcp"], check=True, env=env)
    subprocess.run([str(script), "http://127.0.0.1:9420/mcp"], check=True, env=env)

    data = json.loads(settings.read_text(encoding="utf-8"))
    assert data["theme"] == "light"
    assert data["mcpServers"]["tabula"]["url"] == "http://127.0.0.1:9420/mcp"
    assert "tabula-broker" not in data["mcpServers"]


def test_setup_tabula_wrapper_updates_both_configs(tmp_path: Path) -> None:
    codex_config = tmp_path / "codex.toml"
    claude_settings = tmp_path / "claude.json"
    wrapper = _scripts_dir() / "setup-tabula-mcp.sh"

    env = dict(os.environ)
    env["CODEX_CONFIG_PATH"] = str(codex_config)
    env["CLAUDE_SETTINGS_PATH"] = str(claude_settings)

    subprocess.run([str(wrapper), "http://127.0.0.1:9420/mcp"], check=True, env=env)

    codex_text = codex_config.read_text(encoding="utf-8")
    claude_data = json.loads(claude_settings.read_text(encoding="utf-8"))

    assert "[mcp_servers.tabula]" in codex_text
    assert claude_data["mcpServers"]["tabula"]["url"] == "http://127.0.0.1:9420/mcp"
