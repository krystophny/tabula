from __future__ import annotations

import asyncio
import json
import secrets
import threading
from pathlib import Path

from aiohttp import web

from .canvas_adapter import CanvasAdapter
from .events import CanvasEvent, event_to_payload
from .mcp_server import TabulaMcpServer
from .protocol import _ensure_gitignore

DEFAULT_HOST = "127.0.0.1"
DEFAULT_PORT = 9420


async def broadcast_ws(clients: set[web.WebSocketResponse], message: str) -> None:
    dead: list[web.WebSocketResponse] = []
    for ws in list(clients):
        try:
            await ws.send_str(message)
        except (ConnectionResetError, RuntimeError):
            dead.append(ws)
    for ws in dead:
        clients.discard(ws)


class TabulaServeApp:
    def __init__(self, *, project_dir: Path) -> None:
        self._project_dir = project_dir.resolve()
        _ensure_gitignore(self._project_dir)
        self._ws_clients: set[web.WebSocketResponse] = set()
        self._pending_events: list[CanvasEvent] = []
        self._event_lock = threading.Lock()
        self._mcp_server = TabulaMcpServer(
            CanvasAdapter(
                project_dir=self._project_dir,
                headless=True,
                start_canvas=False,
                on_event=self._queue_event,
            ),
        )

    @property
    def adapter(self) -> CanvasAdapter:
        return self._mcp_server.adapter

    def _queue_event(self, event: CanvasEvent) -> None:
        with self._event_lock:
            self._pending_events.append(event)

    async def _broadcast_pending_events(self) -> None:
        with self._event_lock:
            if not self._pending_events:
                return
            events = self._pending_events[:]
            self._pending_events.clear()
        for event in events:
            payload = json.dumps(event_to_payload(event), separators=(",", ":"))
            await broadcast_ws(self._ws_clients, payload)

    async def handle_mcp_post(self, request: web.Request) -> web.Response:
        try:
            body = await request.json()
        except Exception:
            return web.json_response(
                {"jsonrpc": "2.0", "id": None, "error": {"code": -32700, "message": "parse error"}},
            )

        if not isinstance(body, dict):
            return web.json_response(
                {"jsonrpc": "2.0", "id": None, "error": {"code": -32600, "message": "request must be an object"}},
            )

        response = self._mcp_server.dispatch_message(body)
        await self._broadcast_pending_events()

        if response is None:
            return web.Response(status=202)

        headers: dict[str, str] = {}
        method = body.get("method", "")
        if method == "initialize":
            headers["Mcp-Session-Id"] = secrets.token_hex(16)

        return web.json_response(response, headers=headers)

    async def handle_mcp_get(self, request: web.Request) -> web.StreamResponse:
        response = web.StreamResponse(
            status=200,
            headers={
                "Content-Type": "text/event-stream",
                "Cache-Control": "no-cache",
            },
        )
        await response.prepare(request)
        try:
            while not request.transport.is_closing():
                await response.write(b": keepalive\n\n")
                await asyncio.sleep(30)
        except (ConnectionResetError, asyncio.CancelledError):
            pass
        return response

    async def handle_mcp_delete(self, request: web.Request) -> web.Response:
        return web.Response(status=204)

    async def handle_ws_canvas(self, request: web.Request) -> web.WebSocketResponse:
        ws = web.WebSocketResponse()
        await ws.prepare(request)
        self._ws_clients.add(ws)
        try:
            async for msg in ws:
                if msg.type == web.WSMsgType.TEXT:
                    self.adapter.handle_feedback(msg.data)
                elif msg.type in (web.WSMsgType.ERROR, web.WSMsgType.CLOSE):
                    break
        finally:
            self._ws_clients.discard(ws)
        return ws

    async def handle_files(self, request: web.Request) -> web.Response:
        rel_path = request.match_info.get("path", "")
        if not rel_path:
            return web.Response(status=400, text="missing path")

        file_path = (self._project_dir / rel_path).resolve()
        if not file_path.is_relative_to(self._project_dir):
            return web.Response(status=403, text="access denied")
        if not file_path.is_file():
            return web.Response(status=404, text="not found")

        return web.FileResponse(file_path)

    async def handle_health(self, request: web.Request) -> web.Response:
        return web.json_response({
            "status": "ok",
            "project_dir": str(self._project_dir),
            "sessions": self.adapter.list_sessions(),
            "ws_clients": len(self._ws_clients),
        })

    def create_app(self) -> web.Application:
        app = web.Application()
        app.router.add_post("/mcp", self.handle_mcp_post)
        app.router.add_get("/mcp", self.handle_mcp_get)
        app.router.add_delete("/mcp", self.handle_mcp_delete)
        app.router.add_get("/ws/canvas", self.handle_ws_canvas)
        app.router.add_get("/files/{path:.+}", self.handle_files)
        app.router.add_get("/health", self.handle_health)
        return app


def run_serve(*, project_dir: Path, host: str = DEFAULT_HOST, port: int = DEFAULT_PORT) -> int:
    serve_app = TabulaServeApp(project_dir=project_dir)
    app = serve_app.create_app()
    print(f"tabula serve listening on http://{host}:{port}", flush=True)
    print(f"  MCP endpoint: http://{host}:{port}/mcp", flush=True)
    print(f"  project dir:  {project_dir}", flush=True)
    try:
        web.run_app(app, host=host, port=port, print=None)
    except KeyboardInterrupt:
        pass
    return 0
