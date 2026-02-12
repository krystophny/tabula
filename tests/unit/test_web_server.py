from __future__ import annotations

import asyncio
from pathlib import Path

from aiohttp.test_utils import TestClient, TestServer

from tabula.web.server import TabulaWebApp


async def _make_client(data_dir: Path) -> TestClient:
    app_obj = TabulaWebApp(data_dir=data_dir)
    app = app_obj.create_app()
    return TestClient(TestServer(app))


async def _authenticate(client: TestClient, password: str = "testpassword") -> None:
    await client.post("/api/setup", json={"password": password})


def test_setup_check_initial(tmp_path: Path) -> None:
    async def _run() -> None:
        client = await _make_client(tmp_path)
        async with client:
            resp = await client.get("/api/setup")
            data = await resp.json()
            assert data["has_password"] is False
            assert data["authenticated"] is False

    asyncio.run(_run())


def test_setup_password(tmp_path: Path) -> None:
    async def _run() -> None:
        client = await _make_client(tmp_path)
        async with client:
            resp = await client.post("/api/setup", json={"password": "securepass"})
            assert resp.status == 200
            data = await resp.json()
            assert data["ok"] is True
            assert "tabula_session" in {c.key for c in client.session.cookie_jar}

    asyncio.run(_run())


def test_setup_password_rejects_second_set(tmp_path: Path) -> None:
    async def _run() -> None:
        client = await _make_client(tmp_path)
        async with client:
            await client.post("/api/setup", json={"password": "securepass"})
            resp = await client.post("/api/setup", json={"password": "another"})
            assert resp.status == 409

    asyncio.run(_run())


def test_login_success(tmp_path: Path) -> None:
    async def _run() -> None:
        client = await _make_client(tmp_path)
        async with client:
            await _authenticate(client)
            client.session.cookie_jar.clear()

            resp = await client.post("/api/login", json={"password": "testpassword"})
            assert resp.status == 200

    asyncio.run(_run())


def test_login_failure(tmp_path: Path) -> None:
    async def _run() -> None:
        client = await _make_client(tmp_path)
        async with client:
            await _authenticate(client)
            client.session.cookie_jar.clear()

            resp = await client.post("/api/login", json={"password": "wrongpass"})
            assert resp.status == 401

    asyncio.run(_run())


def test_hosts_crud(tmp_path: Path) -> None:
    async def _run() -> None:
        client = await _make_client(tmp_path)
        async with client:
            await _authenticate(client)

            resp = await client.post("/api/hosts", json={
                "name": "dev",
                "hostname": "10.0.0.1",
                "username": "user",
            })
            assert resp.status == 201
            host = await resp.json()
            assert host["name"] == "dev"
            host_id = host["id"]

            resp = await client.get("/api/hosts")
            assert resp.status == 200
            hosts = await resp.json()
            assert len(hosts) == 1

            resp = await client.get(f"/api/hosts/{host_id}")
            assert resp.status == 200
            h = await resp.json()
            assert h["hostname"] == "10.0.0.1"

            resp = await client.put(f"/api/hosts/{host_id}", json={"hostname": "10.0.0.2"})
            assert resp.status == 200
            h2 = await resp.json()
            assert h2["hostname"] == "10.0.0.2"

            resp = await client.delete(f"/api/hosts/{host_id}")
            assert resp.status == 204

            resp = await client.get("/api/hosts")
            hosts = await resp.json()
            assert len(hosts) == 0

    asyncio.run(_run())


def test_hosts_require_auth(tmp_path: Path) -> None:
    async def _run() -> None:
        client = await _make_client(tmp_path)
        async with client:
            resp = await client.get("/api/hosts")
            assert resp.status == 401

    asyncio.run(_run())


def test_logout(tmp_path: Path) -> None:
    async def _run() -> None:
        client = await _make_client(tmp_path)
        async with client:
            await _authenticate(client)

            resp = await client.get("/api/hosts")
            assert resp.status == 200

            await client.post("/api/logout")

            resp = await client.get("/api/hosts")
            assert resp.status == 401

    asyncio.run(_run())


def test_connect_without_ssh_server(tmp_path: Path) -> None:
    async def _run() -> None:
        client = await _make_client(tmp_path)
        async with client:
            await _authenticate(client)
            await client.post("/api/hosts", json={
                "name": "dev",
                "hostname": "127.0.0.1",
                "port": 59999,
                "username": "user",
            })

            resp = await client.post("/api/connect", json={"host_id": 1})
            assert resp.status == 502

    asyncio.run(_run())


def test_sessions_list(tmp_path: Path) -> None:
    async def _run() -> None:
        client = await _make_client(tmp_path)
        async with client:
            await _authenticate(client)
            resp = await client.get("/api/sessions")
            assert resp.status == 200
            data = await resp.json()
            assert data["sessions"] == []

    asyncio.run(_run())
