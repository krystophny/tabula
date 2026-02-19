from __future__ import annotations

import json
import runpy
import sys
import types
import warnings
from dataclasses import dataclass
from pathlib import Path

import pytest

from tabula.cli import main


@dataclass(frozen=True)
class _FakePaths:
    project_dir: Path
    agents_path: Path
    mcp_config_path: Path


@dataclass(frozen=True)
class _FakeBootstrapResult:
    paths: _FakePaths
    git_initialized: bool
    agents_preserved: bool


def _make_fake_bootstrap(*, resolve: bool = True, git_initialized: bool = False, agents_preserved: bool = False):
    def fake_bootstrap(project_dir: Path):
        base = project_dir.resolve() if resolve else project_dir
        return _FakeBootstrapResult(
            paths=_FakePaths(
                project_dir=base,
                agents_path=base / "AGENTS.md",
                mcp_config_path=base / ".tabula" / "codex-mcp.toml",
            ),
            git_initialized=git_initialized,
            agents_preserved=agents_preserved,
        )

    return fake_bootstrap


def test_given_schema_mode_when_invoked_then_prints_contract(capsys) -> None:
    rc = main(["schema"])
    out = capsys.readouterr().out
    schema = json.loads(out)

    assert rc == 0
    assert schema["title"] == "TabulaCanvasEvent"
    assert len(schema["oneOf"]) == 4


def test_given_canvas_mode_when_invoked_then_calls_ui_runner(monkeypatch) -> None:
    seen: dict[str, object] = {}

    def fake_run_canvas(*, poll_interval_ms: int) -> int:
        seen["poll_ms"] = poll_interval_ms
        return 7

    monkeypatch.setattr("tabula.cli.has_display", lambda _env=None: True)
    monkeypatch.setitem(sys.modules, "tabula.window", types.SimpleNamespace(run_canvas=fake_run_canvas))

    rc = main(["canvas", "--poll-ms", "999"])
    assert rc == 7
    assert seen["poll_ms"] == 999


def test_given_canvas_mode_when_ui_runner_raises_then_error_is_reported(monkeypatch, capsys) -> None:
    monkeypatch.setattr("tabula.cli.has_display", lambda _env=None: True)
    def fake_run_canvas(*, poll_interval_ms: int) -> int:
        raise RuntimeError(f"boom-{poll_interval_ms}")

    monkeypatch.setitem(sys.modules, "tabula.window", types.SimpleNamespace(run_canvas=fake_run_canvas))

    rc = main(["canvas", "--poll-ms", "55"])
    err = capsys.readouterr().err

    assert rc == 2
    assert "failed to start canvas window: boom-55" in err


def test_given_canvas_mode_without_display_then_shows_headless_hint(monkeypatch, capsys) -> None:
    monkeypatch.setattr("tabula.cli.has_display", lambda _env=None: False)
    rc = main(["canvas"])
    err = capsys.readouterr().err

    assert rc == 2
    assert "DISPLAY/WAYLAND_DISPLAY not found" in err


def test_given_bootstrap_mode_when_invoked_then_project_is_prepared(monkeypatch, tmp_path: Path, capsys) -> None:
    monkeypatch.setattr("tabula.cli.bootstrap_project", _make_fake_bootstrap(resolve=False, git_initialized=True))
    rc = main(["bootstrap", "--project-dir", str(tmp_path)])
    out = capsys.readouterr().out

    assert rc == 0
    assert "project prepared:" in out
    assert "tabula sidecar protocol:" in out
    assert "mcp config snippet:" in out
    assert "git initialized" in out


def test_given_bootstrap_runtime_failure_when_invoked_then_error_and_nonzero(monkeypatch, tmp_path: Path, capsys) -> None:
    def fake_bootstrap(_project_dir: Path):
        raise RuntimeError("bootstrap failed hard")

    monkeypatch.setattr("tabula.cli.bootstrap_project", fake_bootstrap)

    rc = main(["bootstrap", "--project-dir", str(tmp_path)])
    err = capsys.readouterr().err
    assert rc == 1
    assert "bootstrap failed hard" in err


def test_given_bootstrap_with_existing_agents_when_invoked_then_preserved_message_printed(
    monkeypatch, tmp_path: Path, capsys
) -> None:
    monkeypatch.setattr(
        "tabula.cli.bootstrap_project",
        _make_fake_bootstrap(resolve=False, agents_preserved=True),
    )
    rc = main(["bootstrap", "--project-dir", str(tmp_path)])
    out = capsys.readouterr().out

    assert rc == 0
    assert "existing AGENTS.md is preserved; tabula protocol is in sidecar" in out


def test_given_mcp_server_mode_when_invoked_then_bootstrap_and_server_runner_are_called(monkeypatch, tmp_path: Path) -> None:
    calls: dict[str, object] = {}

    def fake_run_server(
        *,
        project_dir: Path,
        headless: bool,
        fresh_canvas: bool,
        poll_interval_ms: int,
        start_canvas: bool,
    ) -> int:
        calls["project_dir"] = project_dir
        calls["headless"] = headless
        calls["fresh_canvas"] = fresh_canvas
        calls["poll"] = poll_interval_ms
        calls["start_canvas"] = start_canvas
        return 11

    monkeypatch.setattr("tabula.cli.bootstrap_project", _make_fake_bootstrap())
    monkeypatch.setattr("tabula.cli.run_mcp_stdio_server", fake_run_server)

    rc = main(["mcp-server", "--project-dir", str(tmp_path), "--headless", "--no-canvas", "--poll-ms", "777"])
    assert rc == 11
    assert calls["project_dir"] == tmp_path.resolve()
    assert calls["headless"] is True
    assert calls["fresh_canvas"] is False
    assert calls["poll"] == 777
    assert calls["start_canvas"] is False


def test_given_mcp_server_with_fresh_canvas_flag_when_invoked_then_runner_receives_fresh_canvas(
    monkeypatch, tmp_path: Path
) -> None:
    calls: dict[str, object] = {}

    def fake_run_server(
        *,
        project_dir: Path,
        headless: bool,
        fresh_canvas: bool,
        poll_interval_ms: int,
        start_canvas: bool,
    ) -> int:
        calls["project_dir"] = project_dir
        calls["headless"] = headless
        calls["fresh_canvas"] = fresh_canvas
        calls["poll"] = poll_interval_ms
        calls["start_canvas"] = start_canvas
        return 17

    monkeypatch.setattr("tabula.cli.bootstrap_project", _make_fake_bootstrap())
    monkeypatch.setattr("tabula.cli.run_mcp_stdio_server", fake_run_server)

    rc = main(["mcp-server", "--project-dir", str(tmp_path), "--fresh-canvas"])
    assert rc == 17
    assert calls["fresh_canvas"] is True
    assert calls["headless"] is False
    assert calls["start_canvas"] is True


def test_given_mcp_server_bootstrap_failure_when_invoked_then_nonzero(monkeypatch, tmp_path: Path, capsys) -> None:
    def fake_bootstrap(_project_dir: Path):
        raise RuntimeError("mcp bootstrap failed")

    monkeypatch.setattr("tabula.cli.bootstrap_project", fake_bootstrap)
    rc = main(["mcp-server", "--project-dir", str(tmp_path)])
    err = capsys.readouterr().err
    assert rc == 1
    assert "mcp bootstrap failed" in err


def test_given_mcp_http_bridge_mode_when_invoked_then_bridge_runner_is_called(monkeypatch) -> None:
    calls: dict[str, object] = {}

    def fake_bridge(*, mcp_url: str) -> int:
        calls["mcp_url"] = mcp_url
        return 23

    monkeypatch.setattr("tabula.cli.run_mcp_http_bridge", fake_bridge)

    rc = main(["mcp-http-bridge", "--mcp-url", "http://127.0.0.1:9420/mcp"])

    assert rc == 23
    assert calls["mcp_url"] == "http://127.0.0.1:9420/mcp"


def test_given_run_mode_when_invoked_then_codex_launches_with_inline_mcp_yolo_and_search(monkeypatch, tmp_path: Path) -> None:
    seen: dict[str, object] = {}

    class _RunResult:
        returncode = 19

    def fake_run(cmd, cwd=None):
        seen["cmd"] = cmd
        return _RunResult()

    monkeypatch.setattr("tabula.cli.bootstrap_project", _make_fake_bootstrap())
    monkeypatch.setattr("tabula.cli.subprocess.run", fake_run)

    rc = main(
        [
            "run",
            "--project-dir",
            str(tmp_path),
            "--headless",
            "--no-canvas",
            "--poll-ms",
            "777",
            "hello from tabula run",
        ]
    )

    assert rc == 19
    cmd = seen["cmd"]
    assert isinstance(cmd, list)
    assert cmd[0] == "codex"
    assert "--yolo" in cmd
    assert "--search" in cmd
    assert "--no-alt-screen" in cmd
    assert "hello from tabula run" in cmd

    command_override = cmd[cmd.index("-c") + 1]
    assert "mcp_servers.tabula.command" in command_override
    assert "bash" in command_override
    args_override = cmd[cmd.index("-c", cmd.index("-c") + 1) + 1]
    assert "mcp_servers.tabula.args=" in args_override
    assert "-lc" in args_override
    assert "python" in args_override
    assert "--headless" in args_override
    assert "--no-canvas" in args_override
    assert "--fresh-canvas" in args_override
    assert "777" in args_override


def test_given_run_mode_when_codex_missing_then_nonzero_and_hint(monkeypatch, tmp_path: Path, capsys) -> None:
    def fake_run(_cmd, cwd=None):
        raise FileNotFoundError("codex")

    monkeypatch.setattr("tabula.cli.bootstrap_project", _make_fake_bootstrap())
    monkeypatch.setattr("tabula.cli.subprocess.run", fake_run)

    rc = main(["run", "--project-dir", str(tmp_path)])
    err = capsys.readouterr().err
    assert rc == 1
    assert "codex CLI not found on PATH" in err


def test_given_run_mode_with_claude_assistant_when_invoked_then_claude_launches_with_inline_mcp(
    monkeypatch, tmp_path: Path
) -> None:
    seen: dict[str, object] = {}

    class _RunResult:
        returncode = 23

    def fake_run(cmd, cwd=None):
        seen["cmd"] = cmd
        seen["cwd"] = cwd
        return _RunResult()

    monkeypatch.setattr("tabula.cli.bootstrap_project", _make_fake_bootstrap())
    monkeypatch.setattr("tabula.cli.subprocess.run", fake_run)

    rc = main(
        [
            "run",
            "--assistant",
            "claude",
            "--project-dir",
            str(tmp_path),
            "--headless",
            "--no-canvas",
            "--poll-ms",
            "888",
            "hello from claude tabula run",
        ]
    )

    assert rc == 23
    cmd = seen["cmd"]
    assert isinstance(cmd, list)
    assert cmd[0] == "claude"
    assert "--dangerously-skip-permissions" in cmd
    assert "--mcp-config" in cmd
    assert "hello from claude tabula run" in cmd
    cfg = json.loads(cmd[cmd.index("--mcp-config") + 1])
    server = cfg["mcpServers"]["tabula"]
    assert server["command"] == "bash"
    assert server["args"][0] == "-lc"
    assert "--headless" in server["args"][1]
    assert "--no-canvas" in server["args"][1]
    assert "--fresh-canvas" in server["args"][1]
    assert "888" in server["args"][1]
    assert seen["cwd"] == tmp_path.resolve()


def test_given_run_mode_with_claude_assistant_when_claude_missing_then_nonzero_and_hint(
    monkeypatch, tmp_path: Path, capsys
) -> None:
    def fake_run(_cmd, cwd=None):
        raise FileNotFoundError("claude")

    monkeypatch.setattr("tabula.cli.bootstrap_project", _make_fake_bootstrap())
    monkeypatch.setattr("tabula.cli.subprocess.run", fake_run)

    rc = main(["run", "--assistant", "claude", "--project-dir", str(tmp_path)])
    err = capsys.readouterr().err
    assert rc == 1
    assert "claude CLI not found on PATH" in err


def test_given_run_mode_without_display_when_canvas_expected_then_headless_warning(monkeypatch, tmp_path: Path, capsys) -> None:
    class _RunResult:
        returncode = 0

    monkeypatch.setattr("tabula.cli.bootstrap_project", _make_fake_bootstrap())
    monkeypatch.setattr("tabula.cli.subprocess.run", lambda _cmd, cwd=None: _RunResult())
    monkeypatch.delenv("DISPLAY", raising=False)
    monkeypatch.delenv("WAYLAND_DISPLAY", raising=False)

    rc = main(["run", "--project-dir", str(tmp_path)])
    err = capsys.readouterr().err
    assert rc == 0
    assert "tabula MCP server will run headless" in err


def test_given_run_mode_with_display_env_when_invoked_then_display_vars_are_forwarded(monkeypatch, tmp_path: Path) -> None:
    seen: dict[str, object] = {}

    class _RunResult:
        returncode = 0

    def fake_run(cmd, cwd=None):
        seen["cmd"] = cmd
        return _RunResult()

    monkeypatch.setattr("tabula.cli.bootstrap_project", _make_fake_bootstrap())
    monkeypatch.setattr("tabula.cli.subprocess.run", fake_run)
    monkeypatch.setenv("DISPLAY", ":0")
    monkeypatch.setenv("XAUTHORITY", "/tmp/xauth")

    rc = main(["run", "--project-dir", str(tmp_path)])
    assert rc == 0
    cmd = seen["cmd"]
    assert isinstance(cmd, list)
    args_override = cmd[cmd.index("-c", cmd.index("-c") + 1) + 1]
    assert "DISPLAY=" in args_override
    assert "XAUTHORITY=" in args_override


def test_given_serve_mode_when_invoked_then_bootstrap_and_serve_are_called(monkeypatch, tmp_path: Path) -> None:
    calls: dict[str, object] = {}

    def fake_run_serve(*, project_dir, host, port):
        calls["project_dir"] = project_dir
        calls["host"] = host
        calls["port"] = port
        return 0

    monkeypatch.setattr("tabula.cli.bootstrap_project", _make_fake_bootstrap())
    monkeypatch.setattr("tabula.serve.run_serve", fake_run_serve)

    rc = main(["serve", "--project-dir", str(tmp_path), "--host", "0.0.0.0", "--port", "7777"])
    assert rc == 0
    assert calls["host"] == "0.0.0.0"
    assert calls["port"] == 7777


def test_given_web_mode_when_invoked_then_web_server_is_called(monkeypatch, tmp_path: Path) -> None:
    calls: dict[str, object] = {}

    def fake_run_web(
        *,
        data_dir,
        host,
        port,
        local_project_dir=None,
        local_mcp_url=None,
        ptyd_url=None,
        dev_runtime=False,
    ):
        calls["data_dir"] = data_dir
        calls["host"] = host
        calls["port"] = port
        calls["local_project_dir"] = local_project_dir
        calls["local_mcp_url"] = local_mcp_url
        calls["ptyd_url"] = ptyd_url
        calls["dev_runtime"] = dev_runtime
        return 0

    monkeypatch.setattr("tabula.cli.bootstrap_project", _make_fake_bootstrap())
    monkeypatch.setattr("tabula.web.server.run_web", fake_run_web)

    rc = main(["web", "--data-dir", str(tmp_path), "--port", "7778"])
    assert rc == 0
    assert calls["port"] == 7778
    assert calls["local_project_dir"] is not None
    assert calls["local_mcp_url"] is None
    assert calls["ptyd_url"] is None
    assert calls["dev_runtime"] is False


def test_given_web_mode_with_reload_related_flags_when_invoked_then_flags_are_forwarded(monkeypatch, tmp_path: Path) -> None:
    calls: dict[str, object] = {}

    def fake_run_web(
        *,
        data_dir,
        host,
        port,
        local_project_dir=None,
        local_mcp_url=None,
        ptyd_url=None,
        dev_runtime=False,
    ):
        calls["local_mcp_url"] = local_mcp_url
        calls["ptyd_url"] = ptyd_url
        calls["dev_runtime"] = dev_runtime
        return 0

    monkeypatch.setattr("tabula.cli.bootstrap_project", _make_fake_bootstrap())
    monkeypatch.setattr("tabula.web.server.run_web", fake_run_web)

    rc = main([
        "web",
        "--data-dir", str(tmp_path),
        "--local-mcp-url", "http://127.0.0.1:9520/mcp",
        "--ptyd-url", "http://127.0.0.1:9433",
        "--dev-runtime",
    ])
    assert rc == 0
    assert calls["local_mcp_url"] == "http://127.0.0.1:9520/mcp"
    assert calls["ptyd_url"] == "http://127.0.0.1:9433"
    assert calls["dev_runtime"] is True


def test_given_ptyd_mode_when_invoked_then_ptyd_server_is_called(monkeypatch, tmp_path: Path) -> None:
    calls: dict[str, object] = {}

    def fake_run_ptyd(*, data_dir, host, port):
        calls["data_dir"] = data_dir
        calls["host"] = host
        calls["port"] = port
        return 0

    monkeypatch.setattr("tabula.web.ptyd.run_ptyd", fake_run_ptyd)

    rc = main(["ptyd", "--data-dir", str(tmp_path), "--host", "0.0.0.0", "--port", "9339"])
    assert rc == 0
    assert calls["data_dir"] == tmp_path
    assert calls["host"] == "0.0.0.0"
    assert calls["port"] == 9339


def test_given_run_with_mcp_url_when_invoked_then_uses_http_mcp(monkeypatch, tmp_path: Path) -> None:
    seen: dict[str, object] = {}

    class _RunResult:
        returncode = 0

    def fake_run(cmd, cwd=None):
        seen["cmd"] = cmd
        seen["cwd"] = cwd
        return _RunResult()

    monkeypatch.setattr("tabula.cli.bootstrap_project", _make_fake_bootstrap())
    monkeypatch.setattr("tabula.cli.subprocess.run", fake_run)

    rc = main(["run", "--assistant", "codex", "--mcp-url", "http://localhost:9420/mcp", "--project-dir", str(tmp_path)])
    assert rc == 0
    cmd = seen["cmd"]
    assert isinstance(cmd, list)
    assert "codex" in cmd
    url_cfg = [c for c in cmd if "tabula.url" in c]
    assert len(url_cfg) == 1
    assert "http://localhost:9420/mcp" in url_cfg[0]


def test_given_run_with_mcp_url_and_claude_when_invoked_then_uses_http_mcp_with_dangerous_flag(
    monkeypatch, tmp_path: Path
) -> None:
    seen: dict[str, object] = {}

    class _RunResult:
        returncode = 0

    def fake_run(cmd, cwd=None):
        seen["cmd"] = cmd
        seen["cwd"] = cwd
        return _RunResult()

    monkeypatch.setattr("tabula.cli.bootstrap_project", _make_fake_bootstrap())
    monkeypatch.setattr("tabula.cli.subprocess.run", fake_run)

    rc = main(["run", "--assistant", "claude", "--mcp-url", "http://localhost:9420/mcp", "--project-dir", str(tmp_path)])
    assert rc == 0
    cmd = seen["cmd"]
    assert isinstance(cmd, list)
    assert cmd[0] == "claude"
    assert "--dangerously-skip-permissions" in cmd
    assert "--mcp-config" in cmd
    cfg = json.loads(cmd[cmd.index("--mcp-config") + 1])
    assert cfg["mcpServers"]["tabula"]["url"] == "http://localhost:9420/mcp"


def test_given_no_args_when_invoked_then_help_and_exit_2(capsys) -> None:
    rc = main([])
    err = capsys.readouterr().err
    assert rc == 2
    assert "usage: tabula" in err


def test_given_unknown_command_shape_when_main_dispatches_then_parser_error_branch_returns_2(monkeypatch) -> None:
    class FakeParser:
        def __init__(self) -> None:
            self.errors: list[str] = []

        def print_help(self, _stream) -> None:
            return None

        def parse_args(self, _argv):
            return type("Args", (), {"command": "unexpected"})()

        def error(self, message: str) -> None:
            self.errors.append(message)

    fake_parser = FakeParser()
    monkeypatch.setattr("tabula.cli._build_parser", lambda: fake_parser)

    rc = main(["unexpected"])
    assert rc == 2
    assert fake_parser.errors == ["unknown command"]


def test_given_cli_module_executed_as_main_when_schema_arg_then_main_guard_exits_zero(monkeypatch, capsys) -> None:
    monkeypatch.setattr(sys, "argv", ["tabula", "schema"])
    with warnings.catch_warnings():
        warnings.filterwarnings("ignore", category=RuntimeWarning, message=".*tabula\\.cli.*")
        with pytest.raises(SystemExit) as exc:
            runpy.run_module("tabula.cli", run_name="__main__")

    assert exc.value.code == 0
    out = capsys.readouterr().out
    assert '"title": "TabulaCanvasEvent"' in out
