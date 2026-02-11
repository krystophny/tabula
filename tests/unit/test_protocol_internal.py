from __future__ import annotations

from pathlib import Path
from types import SimpleNamespace

import pytest

import tabula.protocol as protocol


def test_upsert_replaces_existing_protocol_block() -> None:
    existing = (
        "# AGENTS\n\n"
        f"{protocol.AGENTS_PROTOCOL_BEGIN}\n"
        "old block\n"
        f"{protocol.AGENTS_PROTOCOL_END}\n"
        "tail\n"
    )
    block = f"{protocol.AGENTS_PROTOCOL_BEGIN}\nnew block\n{protocol.AGENTS_PROTOCOL_END}\n"
    merged = protocol._upsert_protocol_block(existing, block)
    assert "new block" in merged
    assert "old block" not in merged
    assert "tail" in merged


def test_upsert_with_empty_existing_returns_block() -> None:
    block = f"{protocol.AGENTS_PROTOCOL_BEGIN}\nx\n{protocol.AGENTS_PROTOCOL_END}\n"
    merged = protocol._upsert_protocol_block("   \n", block)
    assert merged == block


def test_ensure_gitignore_idempotent_when_patterns_present(tmp_path: Path) -> None:
    gitignore = tmp_path / ".gitignore"
    gitignore.write_text(
        "\n".join(protocol.GITIGNORE_BINARY_PATTERNS) + "\n",
        encoding="utf-8",
    )
    protocol._ensure_gitignore(tmp_path)
    lines = gitignore.read_text(encoding="utf-8").splitlines()
    assert lines == protocol.GITIGNORE_BINARY_PATTERNS


def test_ensure_gitignore_appends_with_separator(tmp_path: Path) -> None:
    gitignore = tmp_path / ".gitignore"
    gitignore.write_text("custom\n", encoding="utf-8")
    protocol._ensure_gitignore(tmp_path)
    text = gitignore.read_text(encoding="utf-8")
    assert "custom\n\n.tabula/artifacts/*.pdf" in text


def test_bootstrap_raises_when_git_init_fails(tmp_path: Path, monkeypatch) -> None:
    def fake_run(*args, **kwargs):
        return SimpleNamespace(returncode=1, stdout="", stderr="git failed")

    monkeypatch.setattr(protocol.subprocess, "run", fake_run)
    with pytest.raises(RuntimeError, match="git failed"):
        protocol.bootstrap_project(tmp_path)
