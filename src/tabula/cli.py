from __future__ import annotations

import argparse
import json
import os
import shlex
import subprocess
import sys
from pathlib import Path

from .canvas_adapter import has_display
from .events import event_schema
from .mcp_server import run_mcp_stdio_server
from .protocol import bootstrap_project


DISPLAY_ENV_KEYS = (
    "DISPLAY",
    "WAYLAND_DISPLAY",
    "XAUTHORITY",
    "XDG_RUNTIME_DIR",
    "DBUS_SESSION_BUS_ADDRESS",
)


def _build_mcp_shell_command(*, python_bin: str, mcp_args: list[str], env: dict[str, str]) -> str:
    exports: list[str] = []
    for key in DISPLAY_ENV_KEYS:
        value = env.get(key)
        if value:
            exports.append(f"{key}={shlex.quote(value)}")
    argv = " ".join(shlex.quote(part) for part in [python_bin, *mcp_args])
    if exports:
        return " ".join(exports) + " exec " + argv
    return "exec " + argv


def _build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="tabula")
    sub = parser.add_subparsers(dest="command", required=True)

    p_canvas = sub.add_parser("canvas", help="launch canvas window")
    p_canvas.add_argument("--poll-ms", type=int, default=250)

    sub.add_parser("schema", help="print JSON schema")

    p_bootstrap = sub.add_parser("bootstrap", help="initialize tabula protocol files")
    p_bootstrap.add_argument("--project-dir", type=Path, default=Path("."))

    p_mcp = sub.add_parser("mcp-server", help="run tabula-canvas MCP server over stdio")
    p_mcp.add_argument("--project-dir", type=Path, default=Path("."))
    p_mcp.add_argument("--headless", action="store_true")
    p_mcp.add_argument("--no-canvas", action="store_true")
    p_mcp.add_argument("--fresh-canvas", action="store_true")
    p_mcp.add_argument("--poll-ms", type=int, default=250)

    p_serve = sub.add_parser("serve", help="run tabula HTTP daemon with streamable MCP")
    p_serve.add_argument("--project-dir", type=Path, default=Path("."))
    p_serve.add_argument("--host", default="127.0.0.1")
    p_serve.add_argument("--port", type=int, default=9420)

    p_web = sub.add_parser("web", help="launch tabula web server")
    p_web.add_argument("--data-dir", type=Path, default=Path("~/.tabula-web").expanduser())
    p_web.add_argument("--project-dir", type=Path, default=None, help="run local tabula serve for this project (no SSH needed)")
    p_web.add_argument("--host", default="127.0.0.1")
    p_web.add_argument("--port", type=int, default=8420)

    p_run = sub.add_parser("run", help="launch interactive assistant with tabula MCP preconfigured")
    p_run.add_argument("--project-dir", type=Path, default=Path("."))
    p_run.add_argument("--assistant", choices=("codex", "claude"), default="codex")
    p_run.add_argument("--headless", action="store_true")
    p_run.add_argument("--no-canvas", action="store_true")
    p_run.add_argument("--poll-ms", type=int, default=250)
    p_run.add_argument("--mcp-url", default=None, help="use HTTP MCP endpoint URL instead of stdio (e.g. http://localhost:9420/mcp)")
    p_run.add_argument("prompt", nargs="?", default=None)
    return parser


def _cmd_canvas(poll_ms: int) -> int:
    if not has_display():
        print(
            "DISPLAY/WAYLAND_DISPLAY not found; cannot open canvas window. Use tabula run --headless or tabula mcp-server --headless --no-canvas.",
            file=sys.stderr,
        )
        return 2
    try:
        from .window import run_canvas
    except ModuleNotFoundError:
        print(
            "PySide6 is required for 'tabula canvas'. Install with: python -m pip install -e .[gui]",
            file=sys.stderr,
        )
        return 2
    try:
        return run_canvas(poll_interval_ms=poll_ms)
    except Exception as exc:  # pragma: no cover - defensive
        print(f"failed to start canvas window: {exc}", file=sys.stderr)
        return 2


def _cmd_schema() -> int:
    print(json.dumps(event_schema(), indent=2, sort_keys=True))
    return 0


def _cmd_bootstrap(project_dir: Path) -> int:
    try:
        result = bootstrap_project(project_dir)
    except RuntimeError as exc:
        print(str(exc), file=sys.stderr)
        return 1

    print(f"project prepared: {result.paths.project_dir}")
    print(f"agents protocol: {result.paths.agents_path}")
    print(f"tabula sidecar protocol: {(result.paths.project_dir / '.tabula' / 'AGENTS.tabula.md')}")
    print(f"mcp config snippet: {result.paths.mcp_config_path}")
    if result.agents_preserved:
        print("existing AGENTS.md is preserved; tabula protocol is in sidecar")
    if result.git_initialized:
        print("git initialized")
    return 0


def _cmd_mcp_server(project_dir: Path, headless: bool, no_canvas: bool, fresh_canvas: bool, poll_ms: int) -> int:
    try:
        bootstrap = bootstrap_project(project_dir)
    except RuntimeError as exc:
        print(str(exc), file=sys.stderr)
        return 1

    return run_mcp_stdio_server(
        project_dir=bootstrap.paths.project_dir,
        headless=headless,
        fresh_canvas=fresh_canvas,
        poll_interval_ms=poll_ms,
        start_canvas=not no_canvas,
    )


def _launch_assistant(cmd: list[str], *, name: str, prompt: str | None, cwd: Path | None = None) -> int:
    if prompt:
        cmd.append(prompt)
    try:
        return subprocess.run(cmd, cwd=cwd).returncode
    except FileNotFoundError:
        print(f"{name} CLI not found on PATH", file=sys.stderr)
        return 1


def _codex_stdio_cmd(target: Path, mcp_shell: str) -> list[str]:
    return [
        "codex", "--no-alt-screen", "--yolo", "--search",
        "-C", str(target),
        "-c", f"mcp_servers.tabula-canvas.command={json.dumps('bash')}",
        "-c", f"mcp_servers.tabula-canvas.args={json.dumps(['-lc', mcp_shell])}",
    ]


def _claude_stdio_cmd(mcp_shell: str) -> list[str]:
    cfg = {"mcpServers": {"tabula-canvas": {"command": "bash", "args": ["-lc", mcp_shell]}}}
    return ["claude", "--mcp-config", json.dumps(cfg, separators=(",", ":"))]


def _codex_http_cmd(target: Path, mcp_url: str) -> list[str]:
    return [
        "codex", "--no-alt-screen", "--yolo", "--search",
        "-C", str(target),
        "-c", f"mcp_servers.tabula-canvas.url={json.dumps(mcp_url)}",
    ]


def _claude_http_cmd(mcp_url: str) -> list[str]:
    cfg = {"mcpServers": {"tabula-canvas": {"url": mcp_url}}}
    return ["claude", "--mcp-config", json.dumps(cfg, separators=(",", ":"))]


def _cmd_serve(project_dir: Path, host: str, port: int) -> int:
    try:
        bootstrap = bootstrap_project(project_dir)
    except RuntimeError as exc:
        print(str(exc), file=sys.stderr)
        return 1

    try:
        from .serve import run_serve
    except ImportError:
        print(
            "aiohttp is required for 'tabula serve'. Install with: pip install tabula[web]",
            file=sys.stderr,
        )
        return 2

    return run_serve(project_dir=bootstrap.paths.project_dir, host=host, port=port)


def _cmd_web(data_dir: Path, host: str, port: int, project_dir: Path | None) -> int:
    try:
        from .web.server import run_web
    except ImportError:
        print(
            "aiohttp and asyncssh are required for 'tabula web'. Install with: pip install tabula[web]",
            file=sys.stderr,
        )
        return 2

    resolved_project: Path | None = None
    if project_dir is not None:
        try:
            bootstrap = bootstrap_project(project_dir)
        except RuntimeError as exc:
            print(str(exc), file=sys.stderr)
            return 1
        resolved_project = bootstrap.paths.project_dir

    return run_web(data_dir=data_dir, host=host, port=port, local_project_dir=resolved_project)


def _cmd_run(
    project_dir: Path,
    *,
    assistant: str,
    headless: bool,
    no_canvas: bool,
    poll_ms: int,
    mcp_url: str | None,
    prompt: str | None,
) -> int:
    try:
        bootstrap = bootstrap_project(project_dir)
    except RuntimeError as exc:
        print(str(exc), file=sys.stderr)
        return 1

    target = bootstrap.paths.project_dir

    if mcp_url:
        return _dispatch_assistant(assistant, target=target, mcp_url=mcp_url, prompt=prompt)

    mcp_args = [
        "-m",
        "tabula",
        "mcp-server",
        "--project-dir",
        target.as_posix(),
        "--poll-ms",
        str(poll_ms),
    ]
    if headless:
        mcp_args.append("--headless")
    if no_canvas:
        mcp_args.append("--no-canvas")
    mcp_args.append("--fresh-canvas")

    if (not headless) and (not no_canvas):
        if not (os.environ.get("DISPLAY") or os.environ.get("WAYLAND_DISPLAY")):
            print(
                "warning: no DISPLAY/WAYLAND_DISPLAY detected; tabula-canvas will run headless",
                file=sys.stderr,
            )

    mcp_shell = _build_mcp_shell_command(
        python_bin=sys.executable,
        mcp_args=mcp_args,
        env=dict(os.environ),
    )
    return _dispatch_assistant(assistant, target=target, mcp_shell=mcp_shell, prompt=prompt)


def _dispatch_assistant(
    assistant: str,
    *,
    target: Path,
    mcp_shell: str | None = None,
    mcp_url: str | None = None,
    prompt: str | None = None,
) -> int:
    if assistant == "codex":
        if mcp_url:
            cmd = _codex_http_cmd(target, mcp_url)
        else:
            cmd = _codex_stdio_cmd(target, mcp_shell)
        return _launch_assistant(cmd, name="codex", prompt=prompt)
    if assistant == "claude":
        if mcp_url:
            cmd = _claude_http_cmd(mcp_url)
        else:
            cmd = _claude_stdio_cmd(mcp_shell)
        return _launch_assistant(cmd, name="claude", prompt=prompt, cwd=target)

    print(f"unsupported assistant: {assistant}", file=sys.stderr)
    return 1


def main(argv: list[str] | None = None) -> int:
    parser = _build_parser()
    raw_argv = list(sys.argv[1:] if argv is None else argv)
    if not raw_argv:
        parser.print_help(sys.stderr)
        return 2

    args = parser.parse_args(raw_argv)
    if args.command == "canvas":
        return _cmd_canvas(args.poll_ms)
    if args.command == "schema":
        return _cmd_schema()
    if args.command == "bootstrap":
        return _cmd_bootstrap(args.project_dir)
    if args.command == "mcp-server":
        return _cmd_mcp_server(args.project_dir, args.headless, args.no_canvas, args.fresh_canvas, args.poll_ms)
    if args.command == "serve":
        return _cmd_serve(args.project_dir, args.host, args.port)
    if args.command == "web":
        return _cmd_web(args.data_dir, args.host, args.port, args.project_dir)
    if args.command == "run":
        return _cmd_run(
            args.project_dir,
            assistant=args.assistant,
            headless=args.headless,
            no_canvas=args.no_canvas,
            poll_ms=args.poll_ms,
            mcp_url=args.mcp_url,
            prompt=args.prompt,
        )

    parser.error("unknown command")
    return 2


if __name__ == "__main__":
    raise SystemExit(main())
