from __future__ import annotations

import asyncio
import os
import socket
import threading
from pathlib import Path

import pytest

SCREENSHOT_DIR = Path("/tmp/tabula-e2e")
TEST_PASSWORD = "e2e-test-pw-42"


def _free_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


@pytest.fixture(scope="session")
def screenshot_dir() -> Path:
    SCREENSHOT_DIR.mkdir(parents=True, exist_ok=True)
    return SCREENSHOT_DIR


@pytest.fixture(scope="session")
def mock_bin_dir(tmp_path_factory: pytest.TempPathFactory) -> Path:
    bin_dir = tmp_path_factory.mktemp("mock-bin")

    claude_script = bin_dir / "claude"
    claude_script.write_text(
        "#!/usr/bin/env python3\n"
        "import json, sys, urllib.request\n"
        "cfg = None\n"
        "for i, arg in enumerate(sys.argv):\n"
        "    if arg == '--mcp-config' and i + 1 < len(sys.argv):\n"
        "        cfg = json.loads(sys.argv[i + 1])\n"
        "        break\n"
        "if cfg:\n"
        "    url = cfg['mcpServers']['tabula-canvas']['url']\n"
        "    req = urllib.request.Request(\n"
        "        url, method='POST',\n"
        "        headers={'Content-Type': 'application/json'},\n"
        "        data=json.dumps({\n"
        '            "jsonrpc": "2.0", "id": 1, "method": "initialize",\n'
        '            "params": {"protocolVersion": "2024-11-05",\n'
        '                       "capabilities": {},\n'
        '                       "clientInfo": {"name": "mock-claude"}}\n'
        "        }).encode(),\n"
        "    )\n"
        "    urllib.request.urlopen(req, timeout=5)\n"
        "print('MOCK_CLAUDE_OK')\n"
    )
    claude_script.chmod(0o755)

    codex_script = bin_dir / "codex"
    codex_script.write_text(
        "#!/usr/bin/env python3\n"
        "import json, sys, urllib.request\n"
        "url = None\n"
        "for i, arg in enumerate(sys.argv):\n"
        "    if arg == '-c' and i + 1 < len(sys.argv):\n"
        "        val = sys.argv[i + 1]\n"
        "        if 'mcp_servers' in val:\n"
        "            url = val.split('=', 1)[1].strip('\"')\n"
        "            break\n"
        "if url:\n"
        "    req = urllib.request.Request(\n"
        "        url, method='POST',\n"
        "        headers={'Content-Type': 'application/json'},\n"
        "        data=json.dumps({\n"
        '            "jsonrpc": "2.0", "id": 1, "method": "initialize",\n'
        '            "params": {"protocolVersion": "2024-11-05",\n'
        '                       "capabilities": {},\n'
        '                       "clientInfo": {"name": "mock-codex"}}\n'
        "        }).encode(),\n"
        "    )\n"
        "    urllib.request.urlopen(req, timeout=5)\n"
        "print('MOCK_CODEX_OK')\n"
    )
    codex_script.chmod(0o755)

    return bin_dir


@pytest.fixture(scope="session")
def server_info(tmp_path_factory: pytest.TempPathFactory, mock_bin_dir: Path) -> dict:
    from aiohttp import web

    import tabula.web.server as srv_mod
    from tabula.web.server import TabulaWebApp

    daemon_port = _free_port()
    original_daemon_port = srv_mod.DAEMON_PORT
    srv_mod.DAEMON_PORT = daemon_port

    original_path = os.environ.get("PATH", "")
    original_home = os.environ.get("HOME")
    os.environ["PATH"] = f"{mock_bin_dir}:{original_path}"
    test_home = tmp_path_factory.mktemp("e2e-home")
    (test_home / ".bashrc").write_text(
        f"export PATH={mock_bin_dir}:$PATH\n",
        encoding="utf-8",
    )
    os.environ["HOME"] = str(test_home)

    data_dir = tmp_path_factory.mktemp("e2e-data")
    project_dir = tmp_path_factory.mktemp("e2e-project")
    web_port = _free_port()

    loop = asyncio.new_event_loop()
    runner_holder: list[web.AppRunner] = []
    ready = threading.Event()

    async def _boot() -> None:
        web_app = TabulaWebApp(data_dir=data_dir, local_project_dir=project_dir)
        app = web_app.create_app()
        runner = web.AppRunner(app)
        await runner.setup()
        site = web.TCPSite(runner, "127.0.0.1", web_port)
        await site.start()
        runner_holder.append(runner)

    def _thread_main() -> None:
        asyncio.set_event_loop(loop)
        loop.run_until_complete(_boot())
        ready.set()
        loop.run_forever()

    thread = threading.Thread(target=_thread_main, daemon=True)
    thread.start()
    ready.wait(timeout=30)

    yield {
        "base_url": f"http://127.0.0.1:{web_port}",
        "daemon_port": daemon_port,
        "project_dir": str(project_dir),
    }

    if runner_holder:
        future = asyncio.run_coroutine_threadsafe(
            runner_holder[0].cleanup(), loop
        )
        future.result(timeout=10)
    loop.call_soon_threadsafe(loop.stop)
    thread.join(timeout=5)
    loop.close()

    srv_mod.DAEMON_PORT = original_daemon_port
    os.environ["PATH"] = original_path
    if original_home is None:
        os.environ.pop("HOME", None)
    else:
        os.environ["HOME"] = original_home


@pytest.fixture(scope="session")
def base_url(server_info: dict) -> str:
    return server_info["base_url"]
