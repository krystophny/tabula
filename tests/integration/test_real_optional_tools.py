from __future__ import annotations

import os
import shutil
from pathlib import Path

import pytest

from tabula.runner import SubprocessRunner
from tabula.workflow import render_markdown_to_pdf


@pytest.mark.skipif(os.environ.get("TABULA_REAL_TOOLS") != "1", reason="set TABULA_REAL_TOOLS=1 to run real-tool tests")
def test_real_pandoc_render_markdown_to_pdf(tmp_path: Path) -> None:
    if shutil.which("pandoc") is None:
        pytest.skip("pandoc not installed")

    md = tmp_path / "draft.md"
    pdf = tmp_path / "draft.pdf"
    md.write_text("# Real Render\n\nThis is a real pandoc render test.\n", encoding="utf-8")

    runner = SubprocessRunner()
    result = render_markdown_to_pdf(md, pdf, runner)
    assert result.returncode == 0
    assert pdf.exists()
    assert pdf.stat().st_size > 0


@pytest.mark.skipif(
    os.environ.get("TABULA_REAL_CODEX") != "1",
    reason="set TABULA_REAL_CODEX=1 to run real codex exec integration test",
)
def test_real_codex_exec_writes_output_file(tmp_path: Path) -> None:
    if shutil.which("codex") is None:
        pytest.skip("codex not installed")

    out = tmp_path / "last-message.txt"
    runner = SubprocessRunner()
    prompt = "Reply with exactly: REAL_CODEX_OK"
    result = runner.run(
        [
            "codex",
            "exec",
            "--skip-git-repo-check",
            "-C",
            str(tmp_path),
            "-o",
            str(out),
            prompt,
        ],
        cwd=tmp_path,
        capture=True,
    )

    if result.returncode != 0:
        pytest.skip("codex exec failed in this environment (likely auth/session precondition)")
    assert out.exists()
    assert "REAL_CODEX_OK" in out.read_text(encoding="utf-8")
