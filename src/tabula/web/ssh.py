from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path
from typing import Any

import asyncssh

from .store import HostConfig


@dataclass
class SSHSession:
    host_id: int
    conn: asyncssh.SSHClientConnection
    pty: asyncssh.SSHClientProcess | None = None
    tunnel: Any = None


class SSHService:
    def __init__(self) -> None:
        self._sessions: dict[str, SSHSession] = {}

    async def connect(self, host: HostConfig, session_id: str) -> SSHSession:
        if session_id in self._sessions:
            await self.disconnect(session_id)

        known_hosts_path = Path("~/.ssh/known_hosts").expanduser()
        if not known_hosts_path.exists():
            raise ConnectionError(
                f"~/.ssh/known_hosts not found; cannot verify host key for {host.hostname}. "
                "Create the file (ssh-keyscan) or connect manually first."
            )
        kwargs: dict[str, Any] = {
            "host": host.hostname,
            "port": host.port,
            "username": host.username,
            "known_hosts": str(known_hosts_path),
        }
        if host.key_path:
            key_path = Path(host.key_path).expanduser()
            if key_path.exists():
                kwargs["client_keys"] = [str(key_path)]
        conn = await asyncssh.connect(**kwargs)
        session = SSHSession(host_id=host.id, conn=conn)
        self._sessions[session_id] = session
        return session

    async def open_pty(
        self,
        session_id: str,
        *,
        term_type: str = "xterm-256color",
        width: int = 120,
        height: int = 40,
        command: str | None = None,
    ) -> asyncssh.SSHClientProcess:
        session = self._sessions.get(session_id)
        if session is None:
            raise KeyError(f"session {session_id} not found")

        if session.pty is not None:
            session.pty.close()

        process = await session.conn.create_process(
            command,
            term_type=term_type,
            term_size=(width, height),
            encoding=None,
        )
        session.pty = process
        return process

    async def create_tunnel(self, session_id: str, remote_port: int, local_host: str = "127.0.0.1") -> int:
        session = self._sessions.get(session_id)
        if session is None:
            raise KeyError(f"session {session_id} not found")
        listener = await session.conn.forward_local_port(local_host, 0, "127.0.0.1", remote_port)
        session.tunnel = listener
        return listener.get_port()

    async def disconnect(self, session_id: str) -> None:
        session = self._sessions.pop(session_id, None)
        if session is None:
            return
        if session.pty is not None:
            session.pty.close()
        if session.tunnel is not None:
            session.tunnel.close()
        session.conn.close()
        await session.conn.wait_closed()

    async def disconnect_all(self) -> None:
        for sid in list(self._sessions):
            await self.disconnect(sid)

    def get_session(self, session_id: str) -> SSHSession | None:
        return self._sessions.get(session_id)

    def list_sessions(self) -> list[str]:
        return sorted(self._sessions.keys())
