from __future__ import annotations

import asyncio
import os
from pathlib import Path

from tabula.web.pty import LocalPtyTransport, PtyTransport, SshPtyTransport


def test_local_pty_transport_is_pty_transport() -> None:
    assert issubclass(LocalPtyTransport, PtyTransport)


def test_ssh_pty_transport_is_pty_transport() -> None:
    assert issubclass(SshPtyTransport, PtyTransport)


def test_local_pty_echo(tmp_path: Path) -> None:
    async def _run() -> None:
        transport = await LocalPtyTransport.open(str(tmp_path))
        try:
            transport.write(b"echo hello-tabula-test\n")
            output = b""
            for _ in range(50):
                await asyncio.sleep(0.05)
                try:
                    chunk = os.read(transport._fd, 4096)
                    output += chunk
                except BlockingIOError:
                    continue
            assert b"hello-tabula-test" in output
        finally:
            transport.close()

    asyncio.run(_run())


def test_local_pty_resize(tmp_path: Path) -> None:
    async def _run() -> None:
        transport = await LocalPtyTransport.open(str(tmp_path))
        try:
            transport.resize(80, 24)
        finally:
            transport.close()

    asyncio.run(_run())
