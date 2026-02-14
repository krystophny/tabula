from __future__ import annotations

import asyncio
import json
from pathlib import Path

from aiohttp.test_utils import TestClient, TestServer

from tabula.serve import TabulaServeApp


async def _make_client(project_dir: Path) -> TestClient:
    serve_app = TabulaServeApp(project_dir=project_dir)
    app = serve_app.create_app()
    return TestClient(TestServer(app))


def test_health_returns_ok(tmp_path: Path) -> None:
    async def _run() -> None:
        client = await _make_client(tmp_path)
        async with client:
            resp = await client.get("/health")
            assert resp.status == 200
            data = await resp.json()
            assert data["status"] == "ok"
            assert data["project_dir"] == str(tmp_path)
            assert data["sessions"] == []
            assert data["ws_clients"] == 0

    asyncio.run(_run())


def test_mcp_initialize_returns_session_id(tmp_path: Path) -> None:
    async def _run() -> None:
        client = await _make_client(tmp_path)
        async with client:
            resp = await client.post(
                "/mcp",
                json={"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {}},
            )
            assert resp.status == 200
            assert "Mcp-Session-Id" in resp.headers
            data = await resp.json()
            assert data["id"] == 1
            assert data["result"]["serverInfo"]["name"] == "tabula-canvas"

    asyncio.run(_run())


def test_mcp_initialize_negotiates_protocol_version(tmp_path: Path) -> None:
    async def _run() -> None:
        client = await _make_client(tmp_path)
        async with client:
            resp = await client.post(
                "/mcp",
                json={
                    "jsonrpc": "2.0",
                    "id": 1,
                    "method": "initialize",
                    "params": {"protocolVersion": "2024-11-05"},
                },
            )
            data = await resp.json()
            assert data["result"]["protocolVersion"] == "2024-11-05"

            resp2 = await client.post(
                "/mcp",
                json={
                    "jsonrpc": "2.0",
                    "id": 2,
                    "method": "initialize",
                    "params": {"protocolVersion": "2025-03-26"},
                },
            )
            data2 = await resp2.json()
            assert data2["result"]["protocolVersion"] == "2025-03-26"

            resp3 = await client.post(
                "/mcp",
                json={
                    "jsonrpc": "2.0",
                    "id": 3,
                    "method": "initialize",
                    "params": {"protocolVersion": "9999-12-31"},
                },
            )
            data3 = await resp3.json()
            assert data3["result"]["protocolVersion"] == "2025-03-26"

    asyncio.run(_run())


def test_mcp_tools_list(tmp_path: Path) -> None:
    async def _run() -> None:
        client = await _make_client(tmp_path)
        async with client:
            resp = await client.post(
                "/mcp",
                json={"jsonrpc": "2.0", "id": 1, "method": "tools/list", "params": {}},
            )
            data = await resp.json()
            names = [t["name"] for t in data["result"]["tools"]]
            assert "canvas_activate" in names
            assert "canvas_render_text" in names
            assert "canvas_status" in names

    asyncio.run(_run())


def test_mcp_tools_call_render_text(tmp_path: Path) -> None:
    async def _run() -> None:
        client = await _make_client(tmp_path)
        async with client:
            resp = await client.post(
                "/mcp",
                json={
                    "jsonrpc": "2.0",
                    "id": 1,
                    "method": "tools/call",
                    "params": {
                        "name": "canvas_render_text",
                        "arguments": {
                            "session_id": "s1",
                            "title": "draft",
                            "markdown_or_text": "hello",
                        },
                    },
                },
            )
            data = await resp.json()
            assert data["result"]["isError"] is False
            assert data["result"]["structuredContent"]["kind"] == "text_artifact"

    asyncio.run(_run())


def test_mcp_notification_returns_202(tmp_path: Path) -> None:
    async def _run() -> None:
        client = await _make_client(tmp_path)
        async with client:
            resp = await client.post(
                "/mcp",
                json={"jsonrpc": "2.0", "method": "notifications/initialized", "params": {}},
            )
            assert resp.status == 202

    asyncio.run(_run())


def test_mcp_parse_error(tmp_path: Path) -> None:
    async def _run() -> None:
        client = await _make_client(tmp_path)
        async with client:
            resp = await client.post("/mcp", data=b"not json", headers={"Content-Type": "application/json"})
            data = await resp.json()
            assert data["error"]["code"] == -32700

    asyncio.run(_run())


def test_mcp_delete_returns_204(tmp_path: Path) -> None:
    async def _run() -> None:
        client = await _make_client(tmp_path)
        async with client:
            resp = await client.delete("/mcp")
            assert resp.status == 204

    asyncio.run(_run())


def test_ws_canvas_receives_events(tmp_path: Path) -> None:
    async def _run() -> None:
        client = await _make_client(tmp_path)
        async with client:
            ws = await client.ws_connect("/ws/canvas")

            await client.post(
                "/mcp",
                json={
                    "jsonrpc": "2.0",
                    "id": 1,
                    "method": "tools/call",
                    "params": {
                        "name": "canvas_render_text",
                        "arguments": {
                            "session_id": "s1",
                            "title": "draft",
                            "markdown_or_text": "hello ws",
                        },
                    },
                },
            )

            msg = await asyncio.wait_for(ws.receive_str(), timeout=2.0)
            payload = json.loads(msg)
            assert payload["kind"] == "text_artifact"
            assert payload["text"] == "hello ws"

            await ws.close()

    asyncio.run(_run())


def test_ws_canvas_selection_feedback(tmp_path: Path) -> None:
    async def _run() -> None:
        client = await _make_client(tmp_path)
        async with client:
            resp = await client.post(
                "/mcp",
                json={
                    "jsonrpc": "2.0",
                    "id": 1,
                    "method": "tools/call",
                    "params": {
                        "name": "canvas_render_text",
                        "arguments": {
                            "session_id": "s1",
                            "title": "draft",
                            "markdown_or_text": "a\nb\nc",
                        },
                    },
                },
            )
            data = await resp.json()
            event_id = data["result"]["structuredContent"]["artifact_id"]

            ws = await client.ws_connect("/ws/canvas")
            await ws.send_json({
                "kind": "text_selection",
                "event_id": event_id,
                "line_start": 2,
                "line_end": 2,
                "text": "b",
            })
            await asyncio.sleep(0.1)

            resp2 = await client.post(
                "/mcp",
                json={
                    "jsonrpc": "2.0",
                    "id": 2,
                    "method": "tools/call",
                    "params": {"name": "canvas_selection", "arguments": {"session_id": "s1"}},
                },
            )
            data2 = await resp2.json()
            sel = data2["result"]["structuredContent"]["selection"]
            assert sel["has_selection"] is True
            assert sel["text"] == "b"
            assert sel["line_start"] == 2

            await ws.close()

    asyncio.run(_run())


def test_file_serving(tmp_path: Path) -> None:
    test_file = tmp_path / "test.txt"
    test_file.write_text("hello file", encoding="utf-8")

    async def _run() -> None:
        client = await _make_client(tmp_path)
        async with client:
            resp = await client.get("/files/test.txt")
            assert resp.status == 200
            body = await resp.text()
            assert body == "hello file"

    asyncio.run(_run())


def test_file_serving_nested(tmp_path: Path) -> None:
    sub = tmp_path / "sub"
    sub.mkdir()
    (sub / "nested.txt").write_text("nested content", encoding="utf-8")

    async def _run() -> None:
        client = await _make_client(tmp_path)
        async with client:
            resp = await client.get("/files/sub/nested.txt")
            assert resp.status == 200
            body = await resp.text()
            assert body == "nested content"

    asyncio.run(_run())


def test_file_serving_absolute_path_within_project_allowed(tmp_path: Path) -> None:
    test_file = tmp_path / "abs-ok.txt"
    test_file.write_text("absolute path content", encoding="utf-8")

    async def _run() -> None:
        client = await _make_client(tmp_path)
        async with client:
            resp = await client.get(f"/files/{test_file.as_posix()}")
            assert resp.status == 200
            body = await resp.text()
            assert body == "absolute path content"

    asyncio.run(_run())


def test_file_serving_absolute_path_outside_project_blocked(tmp_path: Path) -> None:
    outside = tmp_path.parent / "outside-abs.txt"
    outside.write_text("outside", encoding="utf-8")

    async def _run() -> None:
        client = await _make_client(tmp_path)
        async with client:
            resp = await client.get(f"/files/{outside.as_posix()}")
            assert resp.status == 403

    try:
        asyncio.run(_run())
    finally:
        outside.unlink(missing_ok=True)


def test_file_traversal_blocked(tmp_path: Path) -> None:
    outside = tmp_path.parent / "outside_secret.txt"
    outside.write_text("secret", encoding="utf-8")
    link = tmp_path / "escape"
    link.symlink_to(outside)

    async def _run() -> None:
        client = await _make_client(tmp_path)
        async with client:
            resp = await client.get("/files/escape")
            assert resp.status == 403

    asyncio.run(_run())
    outside.unlink()


def test_file_not_found(tmp_path: Path) -> None:
    async def _run() -> None:
        client = await _make_client(tmp_path)
        async with client:
            resp = await client.get("/files/nonexistent.txt")
            assert resp.status == 404

    asyncio.run(_run())


def test_on_event_callback_fires(tmp_path: Path) -> None:
    from tabula.canvas_adapter import CanvasAdapter

    events_received: list[object] = []
    adapter = CanvasAdapter(
        project_dir=tmp_path,
        headless=True,
        start_canvas=False,
        on_event=events_received.append,
    )
    adapter.canvas_render_text(session_id="s1", title="t", markdown_or_text="x")
    assert len(events_received) == 1
    assert events_received[0].kind == "text_artifact"


def test_dispatch_message_returns_none_for_notification(tmp_path: Path) -> None:
    import io

    from tabula.canvas_adapter import CanvasAdapter
    from tabula.mcp_server import TabulaMcpServer

    adapter = CanvasAdapter(project_dir=tmp_path, headless=True, start_canvas=False)
    server = TabulaMcpServer(adapter, input_stream=io.BytesIO(), output_stream=io.BytesIO())
    result = server.dispatch_message({"jsonrpc": "2.0", "method": "notifications/initialized", "params": {}})
    assert result is None


def test_dispatch_message_returns_error_for_missing_method(tmp_path: Path) -> None:
    import io

    from tabula.canvas_adapter import CanvasAdapter
    from tabula.mcp_server import TabulaMcpServer

    adapter = CanvasAdapter(project_dir=tmp_path, headless=True, start_canvas=False)
    server = TabulaMcpServer(adapter, input_stream=io.BytesIO(), output_stream=io.BytesIO())
    result = server.dispatch_message({"jsonrpc": "2.0", "id": 1})
    assert result is not None
    assert result["error"]["code"] == -32600


def test_handle_feedback_public_api(tmp_path: Path) -> None:
    from tabula.canvas_adapter import CanvasAdapter

    adapter = CanvasAdapter(project_dir=tmp_path, headless=True, start_canvas=False)
    render = adapter.canvas_render_text(session_id="s1", title="t", markdown_or_text="a\nb")
    adapter.handle_feedback(
        json.dumps({
            "kind": "text_selection",
            "event_id": render["artifact_id"],
            "line_start": 1,
            "line_end": 1,
            "text": "a",
        })
    )
    sel = adapter.canvas_selection(session_id="s1")
    assert sel["selection"]["has_selection"] is True
    assert sel["selection"]["text"] == "a"
