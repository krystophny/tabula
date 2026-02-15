from __future__ import annotations

import asyncio
import fcntl
import json
import logging
import os
import signal
import struct
import termios
from abc import ABC, abstractmethod
from collections.abc import Awaitable, Callable
from typing import Any

import aiohttp

_log = logging.getLogger(__name__)


class PtyTransport(ABC):
    """Abstract PTY transport for terminal WebSocket sessions."""

    @abstractmethod
    def write(self, data: bytes) -> None: ...

    @abstractmethod
    def resize(self, cols: int, rows: int) -> None: ...

    @abstractmethod
    def close(self) -> None: ...

    @abstractmethod
    async def reader(self, on_data: Callable[[bytes], Awaitable[None]]) -> None: ...


class LocalPtyTransport(PtyTransport):
    """PTY transport using a local subprocess with proper session control."""

    def __init__(self, master_fd: int, pid: int) -> None:
        self._fd = master_fd
        self._pid = pid

    @classmethod
    async def open(cls, cwd: str) -> LocalPtyTransport:
        if not os.path.isdir(cwd):
            raise FileNotFoundError(f"No such directory: {cwd}")
        pid, master_fd = os.forkpty()
        if pid == 0:
            try:
                os.chdir(cwd)
                os.environ["TERM"] = "xterm-256color"
                shell = os.environ.get("SHELL", "/bin/bash")
                shell_name = os.path.basename(shell)
                os.execvp(shell, [shell_name, "-i"])
            except BaseException:
                os._exit(1)
        os.set_blocking(master_fd, False)
        return cls(master_fd, pid)

    def write(self, data: bytes) -> None:
        os.write(self._fd, data)

    def resize(self, cols: int, rows: int) -> None:
        fcntl.ioctl(self._fd, termios.TIOCSWINSZ, struct.pack("HHHH", rows, cols, 0, 0))

    def close(self) -> None:
        try:
            os.close(self._fd)
        except OSError:
            pass
        try:
            os.kill(self._pid, signal.SIGTERM)
        except ProcessLookupError:
            return
        try:
            reaped, _ = os.waitpid(self._pid, os.WNOHANG)
            if reaped:
                return
        except ChildProcessError:
            return
        pid = self._pid
        try:
            loop = asyncio.get_running_loop()
            loop.run_in_executor(None, os.waitpid, pid, 0)
        except RuntimeError:
            pass

    async def reader(self, on_data: Callable[[bytes], Awaitable[None]]) -> None:
        loop = asyncio.get_running_loop()
        fd = self._fd
        queue: asyncio.Queue[bytes | None] = asyncio.Queue()

        def _on_readable() -> None:
            try:
                data = os.read(fd, 4096)
                queue.put_nowait(data if data else None)
            except OSError:
                queue.put_nowait(None)

        loop.add_reader(fd, _on_readable)
        try:
            while True:
                data = await queue.get()
                if data is None:
                    break
                await on_data(data)
        except (asyncio.CancelledError, ConnectionResetError):
            pass
        finally:
            try:
                loop.remove_reader(fd)
            except (ValueError, OSError):
                pass


class SshPtyTransport(PtyTransport):
    """PTY transport over SSH using asyncssh."""

    def __init__(self, process: Any) -> None:
        self._process = process

    def write(self, data: bytes) -> None:
        self._process.stdin.write(data)

    def resize(self, cols: int, rows: int) -> None:
        self._process.change_terminal_size(cols, rows)

    def close(self) -> None:
        self._process.close()

    async def reader(self, on_data: Callable[[bytes], Awaitable[None]]) -> None:
        try:
            while not self._process.stdout.at_eof():
                data = await self._process.stdout.read(4096)
                if not data:
                    break
                if isinstance(data, bytes):
                    await on_data(data)
                else:
                    await on_data(data.encode("utf-8", errors="replace"))
        except (asyncio.CancelledError, ConnectionResetError):
            pass


class PtydTransport(PtyTransport):
    """PTY transport via external tabula pty daemon over HTTP+WebSocket."""

    def __init__(
        self,
        *,
        client_session: aiohttp.ClientSession,
        ws: aiohttp.ClientWebSocketResponse,
    ) -> None:
        self._client_session = client_session
        self._ws = ws
        self._closed = False
        self._pending_sends: set[asyncio.Task[None]] = set()

    @classmethod
    async def open(
        cls,
        *,
        ptyd_base_url: str,
        session_id: str,
        cwd: str,
        cols: int = 120,
        rows: int = 40,
    ) -> PtydTransport:
        base = ptyd_base_url.rstrip("/")
        client_session = aiohttp.ClientSession()
        try:
            async with client_session.post(
                f"{base}/api/pty/open",
                json={"session_id": session_id, "cwd": cwd, "cols": cols, "rows": rows},
                timeout=aiohttp.ClientTimeout(total=10),
            ) as resp:
                if resp.status != 200:
                    text = await resp.text()
                    raise ConnectionError(f"ptyd open failed: HTTP {resp.status}: {text}")
        except Exception:
            await client_session.close()
            raise

        if base.startswith("https://"):
            ws_url = "wss://" + base[len("https://"):] + f"/ws/pty/{session_id}"
        elif base.startswith("http://"):
            ws_url = "ws://" + base[len("http://"):] + f"/ws/pty/{session_id}"
        else:
            await client_session.close()
            raise ValueError("ptyd_base_url must start with http:// or https://")

        try:
            ws = await client_session.ws_connect(ws_url, timeout=aiohttp.ClientTimeout(total=10))
        except Exception:
            await client_session.close()
            raise
        return cls(client_session=client_session, ws=ws)

    def _spawn_send(self, coro: Awaitable[None]) -> None:
        if self._closed:
            return
        try:
            loop = asyncio.get_running_loop()
        except RuntimeError:
            return
        task = loop.create_task(coro)
        self._pending_sends.add(task)

        def _done(t: asyncio.Task[None]) -> None:
            self._pending_sends.discard(t)

        task.add_done_callback(_done)

    def write(self, data: bytes) -> None:
        if self._closed:
            return
        if isinstance(data, str):
            payload = data.encode("utf-8")
        else:
            payload = data
        self._spawn_send(self._ws.send_bytes(payload))

    def resize(self, cols: int, rows: int) -> None:
        if self._closed:
            return
        payload = json.dumps({"type": "resize", "cols": int(cols), "rows": int(rows)})
        self._spawn_send(self._ws.send_str(payload))

    async def _aclose(self) -> None:
        if self._closed:
            return
        self._closed = True
        for task in list(self._pending_sends):
            task.cancel()
        self._pending_sends.clear()
        try:
            await self._ws.close()
        except Exception:
            pass
        await self._client_session.close()

    def close(self) -> None:
        try:
            loop = asyncio.get_running_loop()
        except RuntimeError:
            return
        loop.create_task(self._aclose())

    async def reader(self, on_data: Callable[[bytes], Awaitable[None]]) -> None:
        try:
            async for msg in self._ws:
                if msg.type == aiohttp.WSMsgType.BINARY:
                    await on_data(bytes(msg.data))
                elif msg.type == aiohttp.WSMsgType.TEXT:
                    await on_data(msg.data.encode("utf-8", errors="replace"))
                elif msg.type in (aiohttp.WSMsgType.CLOSE, aiohttp.WSMsgType.ERROR):
                    break
        except (asyncio.CancelledError, ConnectionResetError):
            pass
        finally:
            await self._aclose()
