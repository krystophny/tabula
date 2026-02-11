from __future__ import annotations

import json
import os
import shlex
import shutil
import subprocess
import sys
import time
import uuid
from pathlib import Path

import pytest

from tabula.protocol import bootstrap_project


def _build_codex_tmux_shell(project_dir: Path, prompt: str) -> str:
    src_path = (Path.cwd() / "src").resolve()
    server_cmd = (
        f"PYTHONPATH={shlex.quote(str(src_path))} "
        f"{shlex.quote(sys.executable)} -m tabula mcp-server "
        f"--project-dir {shlex.quote(project_dir.as_posix())} --headless --no-canvas"
    )
    cmd = [
        "codex",
        "--no-alt-screen",
        "-C",
        str(project_dir),
        "-c",
        'mcp_servers.tabula-canvas.command="bash"',
        "-c",
        f'mcp_servers.tabula-canvas.args=["-lc",{json.dumps(server_cmd)}]',
        prompt,
    ]
    return "cd " + shlex.quote(str(project_dir)) + " && " + " ".join(shlex.quote(token) for token in cmd)


def _capture_pane(session: str) -> str:
    return subprocess.check_output(
        ["tmux", "capture-pane", "-pt", session, "-S", "-400"],
        text=True,
        errors="replace",
    )


def _run_codex_interactive_and_collect(
    *,
    project_dir: Path,
    events_path: Path,
    prompt: str,
    timeout_s: float = 240.0,
    stop_when=None,
) -> tuple[list[dict[str, object]], str]:
    session = "tabula_e2e_" + uuid.uuid4().hex[:8]
    shell = _build_codex_tmux_shell(project_dir, prompt)
    subprocess.run(["tmux", "new-session", "-d", "-s", session, shell], check=True)

    entered_onboarding = False
    pane = ""
    try:
        deadline = time.time() + timeout_s
        while time.time() < deadline:
            pane = _capture_pane(session)

            if "Press enter to continue" in pane and not entered_onboarding:
                subprocess.run(["tmux", "send-keys", "-t", session, "Enter"], check=True)
                entered_onboarding = True

            if "MCP startup incomplete" in pane or "failed to start" in pane:
                raise AssertionError(f"codex MCP startup failed.\nPane tail:\n{pane[-5000:]}")

            if events_path.exists():
                rows = [
                    json.loads(line)
                    for line in events_path.read_text(encoding="utf-8").splitlines()
                    if line.strip()
                ]
                if stop_when is None:
                    done = bool(rows)
                else:
                    done = bool(stop_when(rows, pane))
                if done:
                    return rows, pane

            time.sleep(1.0)

        return [], pane
    finally:
        subprocess.run(["tmux", "send-keys", "-t", session, "/quit", "Enter"], check=False)
        time.sleep(1)
        subprocess.run(["tmux", "kill-session", "-t", session], check=False)


@pytest.mark.skipif(shutil.which("codex") is None, reason="codex CLI not found on PATH")
@pytest.mark.skipif(shutil.which("tmux") is None, reason="tmux is required for automated interactive terminal runs")
def test_real_codex_interactive_loop_renders_text_artifact(tmp_path: Path) -> None:
    if os.environ.get("TABULA_RUN_REAL_CODEX_INTERACTIVE") != "1":
        pytest.skip("set TABULA_RUN_REAL_CODEX_INTERACTIVE=1 to run real interactive codex test")

    project_dir = tmp_path / "project"
    result = bootstrap_project(project_dir)
    events_path = result.paths.events_path
    events_path.write_text("", encoding="utf-8")

    prompt = (
        "Call MCP tool canvas_render_text exactly once with arguments "
        "session_id='real-e2e', title='real-codex-loop', markdown_or_text='hello from real codex interactive loop'. "
        "Do not run shell commands. After the MCP tool call, reply with DONE."
    )

    rows, pane = _run_codex_interactive_and_collect(project_dir=project_dir, events_path=events_path, prompt=prompt)
    text_event = next(
        (row for row in rows if row.get("kind") == "text_artifact" and row.get("title") == "real-codex-loop"),
        None,
    )
    assert text_event is not None, f"expected text artifact not found.\nPane tail:\n{pane[-5000:]}"
    assert "hello from real codex interactive loop" in str(text_event["text"])


@pytest.mark.skipif(shutil.which("codex") is None, reason="codex CLI not found on PATH")
@pytest.mark.skipif(shutil.which("tmux") is None, reason="tmux is required for automated interactive terminal runs")
def test_real_codex_interactive_loop_mode_cycle_render_then_clear(tmp_path: Path) -> None:
    if os.environ.get("TABULA_RUN_REAL_CODEX_INTERACTIVE") != "1":
        pytest.skip("set TABULA_RUN_REAL_CODEX_INTERACTIVE=1 to run real interactive codex test")

    project_dir = tmp_path / "project2"
    result = bootstrap_project(project_dir)
    events_path = result.paths.events_path
    events_path.write_text("", encoding="utf-8")

    prompt = (
        "Call MCP tool canvas_render_text exactly once with "
        "session_id='real-e2e-cycle', title='cycle-artifact', markdown_or_text='cycle body'. "
        "Then call MCP tool canvas_clear exactly once with session_id='real-e2e-cycle' and reason='done'. "
        "Do not run shell commands. After both MCP tool calls, reply with DONE."
    )

    def _done(rows: list[dict[str, object]], _pane: str) -> bool:
        kinds = [row.get("kind") for row in rows]
        return "text_artifact" in kinds and "clear_canvas" in kinds

    rows, pane = _run_codex_interactive_and_collect(
        project_dir=project_dir,
        events_path=events_path,
        prompt=prompt,
        stop_when=_done,
    )
    kinds = [row.get("kind") for row in rows]
    assert "text_artifact" in kinds, f"missing text_artifact event.\nPane tail:\n{pane[-5000:]}"
    assert "clear_canvas" in kinds, f"missing clear_canvas event.\nPane tail:\n{pane[-5000:]}"
