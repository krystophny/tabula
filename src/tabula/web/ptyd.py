from __future__ import annotations

import asyncio
import json
import logging
import secrets
import time
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

from aiohttp import web

from ..serve import _listen_urls
from .pty import LocalPtyTransport

_log = logging.getLogger(__name__)

DEFAULT_HOST = "127.0.0.1"
DEFAULT_PORT = 9333


@dataclass
class PtySessionRecord:
    session_id: str
    cwd: str
    transport: LocalPtyTransport
    created_at: float = field(default_factory=time.time)
    clients: set[web.WebSocketResponse] = field(default_factory=set)
    reader_task: asyncio.Task[None] | None = None


class PtyDaemonApp:
    def __init__(self, *, data_dir: Path) -> None:
        self._data_dir = data_dir.resolve()
        self._data_dir.mkdir(parents=True, exist_ok=True)
        self._sessions: dict[str, PtySessionRecord] = {}
        self._lock = asyncio.Lock()

    async def _broadcast(self, record: PtySessionRecord, payload: bytes) -> None:
        dead: list[web.WebSocketResponse] = []
        for ws in list(record.clients):
            try:
                await ws.send_bytes(payload)
            except (ConnectionResetError, RuntimeError):
                dead.append(ws)
        for ws in dead:
            record.clients.discard(ws)

    async def _reader_loop(self, record: PtySessionRecord) -> None:
        async def _on_data(data: bytes) -> None:
            await self._broadcast(record, data)

        try:
            await record.transport.reader(_on_data)
        except Exception as exc:  # pragma: no cover - defensive
            _log.warning("pty session %s reader ended: %s", record.session_id, exc)
        finally:
            async with self._lock:
                current = self._sessions.get(record.session_id)
                if current is record:
                    self._sessions.pop(record.session_id, None)
            for ws in list(record.clients):
                try:
                    await ws.close()
                except Exception:
                    pass

    async def _ensure_session(self, *, session_id: str, cwd: str) -> tuple[PtySessionRecord, bool]:
        async with self._lock:
            existing = self._sessions.get(session_id)
            if existing is not None:
                return existing, False

            transport = await LocalPtyTransport.open(cwd)
            record = PtySessionRecord(session_id=session_id, cwd=cwd, transport=transport)
            record.reader_task = asyncio.create_task(self._reader_loop(record))
            self._sessions[session_id] = record
            return record, True

    async def handle_open(self, request: web.Request) -> web.Response:
        try:
            body = await request.json()
        except Exception:
            raise web.HTTPBadRequest(text="invalid JSON")

        session_id = body.get("session_id")
        if not isinstance(session_id, str) or not session_id.strip():
            raise web.HTTPBadRequest(text="session_id required")
        session_id = session_id.strip()

        cwd_obj = body.get("cwd")
        if not isinstance(cwd_obj, str) or not cwd_obj.strip():
            raise web.HTTPBadRequest(text="cwd required")
        cwd = cwd_obj.strip()

        record, created = await self._ensure_session(session_id=session_id, cwd=cwd)

        cols = body.get("cols")
        rows = body.get("rows")
        try:
            if isinstance(cols, int) and isinstance(rows, int) and cols > 0 and rows > 0:
                record.transport.resize(cols, rows)
        except Exception:
            pass

        return web.json_response(
            {
                "session_id": record.session_id,
                "created": created,
                "created_at": record.created_at,
            }
        )

    async def handle_close(self, request: web.Request) -> web.Response:
        try:
            body = await request.json()
        except Exception:
            raise web.HTTPBadRequest(text="invalid JSON")

        session_id = body.get("session_id")
        if not isinstance(session_id, str) or not session_id.strip():
            raise web.HTTPBadRequest(text="session_id required")

        async with self._lock:
            record = self._sessions.pop(session_id, None)
        if record is None:
            return web.json_response({"closed": False, "reason": "not_found"}, status=404)

        try:
            record.transport.close()
        except Exception:
            pass

        if record.reader_task is not None:
            record.reader_task.cancel()

        for ws in list(record.clients):
            try:
                await ws.close()
            except Exception:
                pass

        return web.json_response({"closed": True, "session_id": session_id})

    async def handle_list(self, request: web.Request) -> web.Response:
        sessions: list[dict[str, Any]] = []
        async with self._lock:
            for record in self._sessions.values():
                sessions.append(
                    {
                        "session_id": record.session_id,
                        "cwd": record.cwd,
                        "clients": len(record.clients),
                        "created_at": record.created_at,
                    }
                )
        sessions.sort(key=lambda row: row["created_at"])
        return web.json_response({"sessions": sessions})

    async def handle_health(self, request: web.Request) -> web.Response:
        return web.json_response({"status": "ok", "sessions": len(self._sessions)})

    async def handle_ws(self, request: web.Request) -> web.WebSocketResponse:
        session_id = request.match_info.get("session_id", "")
        if not session_id:
            raise web.HTTPBadRequest(text="session_id missing")

        async with self._lock:
            record = self._sessions.get(session_id)
        if record is None:
            raise web.HTTPNotFound(text="session not found; call /api/pty/open first")

        ws = web.WebSocketResponse()
        await ws.prepare(request)

        record.clients.add(ws)
        try:
            async for msg in ws:
                if msg.type == web.WSMsgType.BINARY:
                    try:
                        record.transport.write(bytes(msg.data))
                    except Exception:
                        break
                elif msg.type == web.WSMsgType.TEXT:
                    try:
                        payload = json.loads(msg.data)
                    except json.JSONDecodeError:
                        try:
                            record.transport.write(msg.data.encode("utf-8"))
                        except Exception:
                            break
                        continue

                    if isinstance(payload, dict) and payload.get("type") == "resize":
                        cols = payload.get("cols")
                        rows = payload.get("rows")
                        if isinstance(cols, int) and isinstance(rows, int) and cols > 0 and rows > 0:
                            try:
                                record.transport.resize(cols, rows)
                            except Exception:
                                pass
                    else:
                        try:
                            record.transport.write(msg.data.encode("utf-8"))
                        except Exception:
                            break
                elif msg.type in (web.WSMsgType.ERROR, web.WSMsgType.CLOSE):
                    break
        finally:
            record.clients.discard(ws)

        return ws

    async def on_shutdown(self, app: web.Application) -> None:
        async with self._lock:
            records = list(self._sessions.values())
            self._sessions.clear()

        for record in records:
            try:
                record.transport.close()
            except Exception:
                pass
            if record.reader_task is not None:
                record.reader_task.cancel()
            for ws in list(record.clients):
                try:
                    await ws.close()
                except Exception:
                    pass

    def create_app(self) -> web.Application:
        app = web.Application()
        app.on_shutdown.append(self.on_shutdown)
        app.router.add_post("/api/pty/open", self.handle_open)
        app.router.add_post("/api/pty/close", self.handle_close)
        app.router.add_get("/api/pty/list", self.handle_list)
        app.router.add_get("/api/health", self.handle_health)
        app.router.add_get("/ws/pty/{session_id}", self.handle_ws)
        return app


def run_ptyd(*, data_dir: Path, host: str = DEFAULT_HOST, port: int = DEFAULT_PORT) -> int:
    app_obj = PtyDaemonApp(data_dir=data_dir)
    app = app_obj.create_app()
    urls = _listen_urls(host, port)
    boot_id = secrets.token_hex(8)
    print("tabula ptyd listening on:", flush=True)
    for url in urls:
        print(f"  {url}", flush=True)
    print(f"  data dir: {data_dir}", flush=True)
    print(f"  boot id:  {boot_id}", flush=True)
    try:
        web.run_app(app, host=host, port=port, print=None)
    except KeyboardInterrupt:
        pass
    return 0
