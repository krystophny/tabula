from __future__ import annotations

import asyncio
import json
import logging
import secrets
import shlex
import sqlite3
from pathlib import Path
from typing import Any

import aiohttp
from aiohttp import web

from ..serve import broadcast_ws
from .pty import LocalPtyTransport, PtyTransport, SshPtyTransport
from .ssh import SSHService
from .store import Store

_log = logging.getLogger(__name__)

DEFAULT_HOST = "127.0.0.1"
DEFAULT_PORT = 8420
SESSION_COOKIE = "tabula_session"
DAEMON_PORT = 9420
DAEMON_STARTUP_TIMEOUT = 10.0
DAEMON_HEALTH_POLL_INTERVAL = 0.5
LOCAL_SESSION_ID = "local"


class TabulaWebApp:
    def __init__(self, *, data_dir: Path, local_project_dir: Path | None = None) -> None:
        self._data_dir = data_dir.resolve()
        self._data_dir.mkdir(parents=True, exist_ok=True)
        self._store = Store(self._data_dir / "tabula.db")
        self._ssh = SSHService()
        self._sessions: dict[str, dict[str, Any]] = {}
        self._terminal_ws: dict[str, web.WebSocketResponse] = {}
        self._canvas_ws: dict[str, set[web.WebSocketResponse]] = {}
        self._tunnel_ports: dict[str, int] = {}
        self._canvas_relay_tasks: dict[str, asyncio.Task[None]] = {}
        self._remote_canvas_ws: dict[str, aiohttp.ClientWebSocketResponse] = {}
        self._static_dir = Path(__file__).parent / "static"
        self._local_project_dir = local_project_dir
        self._local_serve_app = None
        self._local_serve_runner: web.AppRunner | None = None

    @property
    def store(self) -> Store:
        return self._store

    def _new_session_token(self) -> str:
        return secrets.token_hex(32)

    def _check_auth(self, request: web.Request) -> bool:
        token = request.cookies.get(SESSION_COOKIE, "")
        return token in self._sessions

    def _require_auth(self, request: web.Request) -> None:
        if not self._check_auth(request):
            raise web.HTTPUnauthorized(text="unauthorized")

    @staticmethod
    def _parse_host_id(request: web.Request) -> int:
        try:
            return int(request.match_info["id"])
        except (ValueError, KeyError):
            raise web.HTTPBadRequest(text="invalid host id")

    async def handle_setup_check(self, request: web.Request) -> web.Response:
        result: dict[str, Any] = {
            "has_password": self._store.has_admin_password(),
            "authenticated": self._check_auth(request),
        }
        if self._local_project_dir:
            result["local_session"] = LOCAL_SESSION_ID
        return web.json_response(result)

    async def handle_setup_password(self, request: web.Request) -> web.Response:
        if self._store.has_admin_password():
            raise web.HTTPConflict(text="admin password already set")
        body = await request.json()
        password = body.get("password", "")
        try:
            self._store.set_admin_password(password)
        except ValueError as exc:
            raise web.HTTPBadRequest(text=str(exc))
        token = self._new_session_token()
        self._sessions[token] = {"role": "admin"}
        resp = web.json_response({"ok": True})
        resp.set_cookie(SESSION_COOKIE, token, httponly=True, samesite="Strict", secure=request.secure)
        return resp

    async def handle_login(self, request: web.Request) -> web.Response:
        body = await request.json()
        password = body.get("password", "")
        if not self._store.verify_admin_password(password):
            await asyncio.sleep(1)
            _log.warning("failed login attempt from %s", request.remote)
            raise web.HTTPUnauthorized(text="invalid password")
        token = self._new_session_token()
        self._sessions[token] = {"role": "admin"}
        resp = web.json_response({"ok": True})
        resp.set_cookie(SESSION_COOKIE, token, httponly=True, samesite="Strict", secure=request.secure)
        return resp

    async def handle_logout(self, request: web.Request) -> web.Response:
        token = request.cookies.get(SESSION_COOKIE, "")
        self._sessions.pop(token, None)
        resp = web.json_response({"ok": True})
        resp.del_cookie(SESSION_COOKIE)
        return resp

    async def handle_hosts_list(self, request: web.Request) -> web.Response:
        self._require_auth(request)
        hosts = self._store.list_hosts()
        return web.json_response([self._store.host_to_dict(h) for h in hosts])

    async def handle_hosts_create(self, request: web.Request) -> web.Response:
        self._require_auth(request)
        body = await request.json()
        try:
            host = self._store.add_host(
                name=body.get("name", ""),
                hostname=body.get("hostname", ""),
                port=body.get("port", 22),
                username=body.get("username", ""),
                key_path=body.get("key_path", ""),
                project_dir=body.get("project_dir", "~"),
            )
        except (ValueError, sqlite3.IntegrityError) as exc:
            raise web.HTTPBadRequest(text=str(exc))
        return web.json_response(self._store.host_to_dict(host), status=201)

    async def handle_hosts_get(self, request: web.Request) -> web.Response:
        self._require_auth(request)
        host_id = self._parse_host_id(request)
        try:
            host = self._store.get_host(host_id)
        except KeyError:
            raise web.HTTPNotFound(text="host not found")
        return web.json_response(self._store.host_to_dict(host))

    _HOST_UPDATE_FIELDS = {"name", "hostname", "port", "username", "key_path", "project_dir"}

    async def handle_hosts_update(self, request: web.Request) -> web.Response:
        self._require_auth(request)
        host_id = self._parse_host_id(request)
        body = await request.json()
        updates = {k: v for k, v in body.items() if k in self._HOST_UPDATE_FIELDS}
        try:
            host = self._store.update_host(host_id, **updates)
        except KeyError:
            raise web.HTTPNotFound(text="host not found")
        except ValueError as exc:
            raise web.HTTPBadRequest(text=str(exc))
        return web.json_response(self._store.host_to_dict(host))

    async def handle_hosts_delete(self, request: web.Request) -> web.Response:
        self._require_auth(request)
        host_id = self._parse_host_id(request)
        self._store.delete_host(host_id)
        return web.Response(status=204)

    async def handle_connect(self, request: web.Request) -> web.Response:
        self._require_auth(request)
        body = await request.json()
        host_id = body.get("host_id")
        if host_id is None:
            raise web.HTTPBadRequest(text="host_id required")
        try:
            host = self._store.get_host(host_id)
        except KeyError:
            raise web.HTTPNotFound(text="host not found")
        session_id = secrets.token_hex(8)
        try:
            await self._ssh.connect(host, session_id)
        except Exception as exc:
            _log.error("SSH connection to host %d failed: %s", host.id, exc)
            raise web.HTTPBadGateway(text="SSH connection failed")
        return web.json_response({"session_id": session_id, "host": self._store.host_to_dict(host)})

    async def handle_disconnect(self, request: web.Request) -> web.Response:
        self._require_auth(request)
        body = await request.json()
        session_id = body.get("session_id", "")
        task = self._canvas_relay_tasks.pop(session_id, None)
        if task is not None:
            task.cancel()
        await self._ssh.disconnect(session_id)
        self._tunnel_ports.pop(session_id, None)
        self._canvas_ws.pop(session_id, None)
        self._remote_canvas_ws.pop(session_id, None)
        return web.json_response({"ok": True})

    async def _create_pty_transport(self, session_id: str) -> PtyTransport:
        if session_id == LOCAL_SESSION_ID and self._local_project_dir:
            return await LocalPtyTransport.open(str(self._local_project_dir))
        ssh_session = self._ssh.get_session(session_id)
        if ssh_session is None:
            raise web.HTTPNotFound(text="session not found")
        process = await self._ssh.open_pty(session_id)
        return SshPtyTransport(process)

    async def handle_terminal_ws(self, request: web.Request) -> web.WebSocketResponse:
        if not self._check_auth(request):
            raise web.HTTPUnauthorized(text="unauthorized")

        session_id = request.match_info["session_id"]
        ws = web.WebSocketResponse()
        await ws.prepare(request)
        self._terminal_ws[session_id] = ws

        transport = await self._create_pty_transport(session_id)
        read_task = asyncio.create_task(transport.reader(ws))

        try:
            async for msg in ws:
                if msg.type == aiohttp.WSMsgType.BINARY:
                    transport.write(msg.data)
                elif msg.type == aiohttp.WSMsgType.TEXT:
                    try:
                        cmd = json.loads(msg.data)
                    except json.JSONDecodeError:
                        transport.write(msg.data.encode("utf-8"))
                        continue
                    if cmd.get("type") == "resize":
                        transport.resize(cmd.get("cols", 120), cmd.get("rows", 40))
                    else:
                        transport.write(msg.data.encode("utf-8"))
                elif msg.type in (aiohttp.WSMsgType.ERROR, aiohttp.WSMsgType.CLOSE):
                    break
        finally:
            read_task.cancel()
            transport.close()
            self._terminal_ws.pop(session_id, None)

        return ws

    async def handle_start_daemon(self, request: web.Request) -> web.Response:
        self._require_auth(request)
        body = await request.json()
        session_id = body.get("session_id", "")
        ssh_session = self._ssh.get_session(session_id)
        if ssh_session is None:
            raise web.HTTPNotFound(text="session not found")

        host = self._store.get_host(ssh_session.host_id)
        project_dir = host.project_dir

        safe_dir = shlex.quote(project_dir)
        await ssh_session.conn.run(
            f"cd {safe_dir} && nohup python3 -m tabula serve --port {DAEMON_PORT} > /tmp/tabula-serve.log 2>&1 &",
            check=False,
            timeout=5,
        )

        tunnel_port = await self._ssh.create_tunnel(session_id, DAEMON_PORT)
        self._tunnel_ports[session_id] = tunnel_port

        healthy = False
        loop = asyncio.get_running_loop()
        deadline = loop.time() + DAEMON_STARTUP_TIMEOUT
        async with aiohttp.ClientSession() as cs:
            while loop.time() < deadline:
                try:
                    async with cs.get(f"http://127.0.0.1:{tunnel_port}/health", timeout=aiohttp.ClientTimeout(total=2)) as resp:
                        if resp.status == 200:
                            healthy = True
                            break
                except Exception:
                    pass
                await asyncio.sleep(DAEMON_HEALTH_POLL_INTERVAL)

        if not healthy:
            raise web.HTTPBadGateway(text="remote daemon did not start in time")

        self._start_canvas_relay(session_id, tunnel_port)
        return web.json_response({"tunnel_port": tunnel_port, "status": "running"})

    async def handle_canvas_ws(self, request: web.Request) -> web.WebSocketResponse:
        if not self._check_auth(request):
            raise web.HTTPUnauthorized(text="unauthorized")

        session_id = request.match_info["session_id"]
        ws = web.WebSocketResponse()
        await ws.prepare(request)

        if session_id not in self._canvas_ws:
            self._canvas_ws[session_id] = set()
        self._canvas_ws[session_id].add(ws)

        try:
            async for msg in ws:
                if msg.type == aiohttp.WSMsgType.TEXT:
                    remote_ws = self._remote_canvas_ws.get(session_id)
                    if remote_ws and not remote_ws.closed:
                        await remote_ws.send_str(msg.data)
                elif msg.type in (aiohttp.WSMsgType.ERROR, aiohttp.WSMsgType.CLOSE):
                    break
        finally:
            clients = self._canvas_ws.get(session_id)
            if clients:
                clients.discard(ws)

        return ws

    async def handle_file_proxy(self, request: web.Request) -> web.Response:
        self._require_auth(request)
        session_id = request.match_info["session_id"]
        file_path = request.match_info["path"]
        if ".." in file_path or file_path.startswith("/") or "\x00" in file_path:
            raise web.HTTPForbidden(text="invalid path")
        tunnel_port = self._tunnel_ports.get(session_id)
        if not tunnel_port:
            raise web.HTTPNotFound(text="no active tunnel for session")

        url = f"http://127.0.0.1:{tunnel_port}/files/{file_path}"
        try:
            async with aiohttp.ClientSession() as cs:
                async with cs.get(url, timeout=aiohttp.ClientTimeout(total=30)) as resp:
                    if resp.status != 200:
                        return web.Response(
                            status=resp.status, text=await resp.text(),
                            content_type=resp.content_type or "text/plain",
                        )
                    body = await resp.read()
                    return web.Response(body=body, content_type=resp.content_type or "application/octet-stream")
        except Exception as exc:
            _log.error("file fetch through tunnel failed: %s", exc)
            raise web.HTTPBadGateway(text="file fetch failed")

    def _start_canvas_relay(self, session_id: str, tunnel_port: int) -> None:
        old_task = self._canvas_relay_tasks.pop(session_id, None)
        if old_task is not None:
            old_task.cancel()
        task = asyncio.create_task(self._canvas_relay_loop(session_id, tunnel_port))
        self._canvas_relay_tasks[session_id] = task

    async def _canvas_relay_loop(self, session_id: str, tunnel_port: int) -> None:
        url = f"http://127.0.0.1:{tunnel_port}/ws/canvas"
        try:
            async with aiohttp.ClientSession() as cs:
                async with cs.ws_connect(url) as remote_ws:
                    self._remote_canvas_ws[session_id] = remote_ws
                    async for msg in remote_ws:
                        if msg.type == aiohttp.WSMsgType.TEXT:
                            clients = self._canvas_ws.get(session_id, set())
                            await broadcast_ws(clients, msg.data)
                        elif msg.type in (aiohttp.WSMsgType.ERROR, aiohttp.WSMsgType.CLOSE):
                            break
        except (asyncio.CancelledError, aiohttp.ClientError):
            pass
        finally:
            self._remote_canvas_ws.pop(session_id, None)

    async def handle_sessions_list(self, request: web.Request) -> web.Response:
        self._require_auth(request)
        result: dict[str, Any] = {"sessions": self._ssh.list_sessions()}
        if self._local_project_dir:
            result["local_session"] = {
                "session_id": LOCAL_SESSION_ID,
                "project_dir": str(self._local_project_dir),
                "mcp_url": f"http://127.0.0.1:{DAEMON_PORT}/mcp",
            }
        return web.json_response(result)

    async def _start_local_serve(self, app: web.Application) -> None:
        if self._local_project_dir is None:
            return
        from ..serve import TabulaServeApp
        self._local_serve_app = TabulaServeApp(project_dir=self._local_project_dir)
        serve_app = self._local_serve_app.create_app()
        runner = web.AppRunner(serve_app)
        await runner.setup()
        try:
            site = web.TCPSite(runner, "127.0.0.1", DAEMON_PORT)
            await site.start()
        except OSError as exc:
            await runner.cleanup()
            _log.error("failed to start local serve on port %d: %s", DAEMON_PORT, exc)
            raise RuntimeError(
                f"port {DAEMON_PORT} already in use; is another tabula serve running?"
            ) from exc
        self._local_serve_runner = runner
        self._tunnel_ports[LOCAL_SESSION_ID] = DAEMON_PORT
        self._start_canvas_relay(LOCAL_SESSION_ID, DAEMON_PORT)

    async def _on_shutdown(self, app: web.Application) -> None:
        for task in self._canvas_relay_tasks.values():
            task.cancel()
        if self._local_serve_runner is not None:
            await self._local_serve_runner.cleanup()
        await self._ssh.disconnect_all()
        self._store.close()

    async def _serve_index(self, request: web.Request) -> web.Response:
        index = self._static_dir / "index.html"
        if index.exists():
            return web.FileResponse(index)
        return web.Response(status=404, text="web client not found")

    @staticmethod
    async def _security_headers(_request: web.Request, response: web.StreamResponse) -> None:
        response.headers["X-Frame-Options"] = "DENY"
        response.headers["Content-Security-Policy"] = (
            "default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self'"
        )

    def create_app(self) -> web.Application:
        app = web.Application()
        app.on_response_prepare.append(self._security_headers)
        app.on_startup.append(self._start_local_serve)
        app.on_shutdown.append(self._on_shutdown)

        app.router.add_get("/api/setup", self.handle_setup_check)
        app.router.add_post("/api/setup", self.handle_setup_password)
        app.router.add_post("/api/login", self.handle_login)
        app.router.add_post("/api/logout", self.handle_logout)

        app.router.add_get("/api/hosts", self.handle_hosts_list)
        app.router.add_post("/api/hosts", self.handle_hosts_create)
        app.router.add_get("/api/hosts/{id}", self.handle_hosts_get)
        app.router.add_put("/api/hosts/{id}", self.handle_hosts_update)
        app.router.add_delete("/api/hosts/{id}", self.handle_hosts_delete)

        app.router.add_post("/api/connect", self.handle_connect)
        app.router.add_post("/api/disconnect", self.handle_disconnect)
        app.router.add_get("/api/sessions", self.handle_sessions_list)

        app.router.add_post("/api/daemon/start", self.handle_start_daemon)

        app.router.add_get("/ws/terminal/{session_id}", self.handle_terminal_ws)
        app.router.add_get("/ws/canvas/{session_id}", self.handle_canvas_ws)
        app.router.add_get("/api/files/{session_id}/{path:.+}", self.handle_file_proxy)

        if self._static_dir.is_dir():
            app.router.add_get("/", self._serve_index)
            app.router.add_static("/static/", self._static_dir, show_index=False)

        return app


def run_web(
    *,
    data_dir: Path,
    host: str = DEFAULT_HOST,
    port: int = DEFAULT_PORT,
    local_project_dir: Path | None = None,
) -> int:
    web_app = TabulaWebApp(data_dir=data_dir, local_project_dir=local_project_dir)
    app = web_app.create_app()
    display_host = "localhost" if host in ("127.0.0.1", "0.0.0.0") else host
    url = f"http://{display_host}:{port}"
    print(f"tabula web: {url}", flush=True)
    if local_project_dir:
        print(f"  local project: {local_project_dir}", flush=True)
        print(f"  local MCP:     http://127.0.0.1:{DAEMON_PORT}/mcp", flush=True)
    try:
        web.run_app(app, host=host, port=port, print=None)
    except KeyboardInterrupt:
        pass
    return 0
