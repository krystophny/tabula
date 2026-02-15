from __future__ import annotations

import json
import sys
import urllib.error
import urllib.request
from typing import Any


def _post_json(mcp_url: str, payload: Any) -> dict[str, Any]:
    body = json.dumps(payload, separators=(",", ":")).encode("utf-8")
    req = urllib.request.Request(
        mcp_url,
        method="POST",
        headers={"Content-Type": "application/json"},
        data=body,
    )
    with urllib.request.urlopen(req, timeout=30) as resp:
        raw = resp.read()
    decoded = json.loads(raw.decode("utf-8"))
    if not isinstance(decoded, dict):
        raise ValueError("MCP response is not a JSON object")
    return decoded


def _emit(payload: dict[str, Any]) -> None:
    sys.stdout.write(json.dumps(payload, separators=(",", ":")))
    sys.stdout.write("\n")
    sys.stdout.flush()


def run_mcp_http_bridge(*, mcp_url: str) -> int:
    for raw_line in sys.stdin:
        line = raw_line.strip()
        if not line:
            continue

        try:
            request_obj: Any = json.loads(line)
        except json.JSONDecodeError as exc:
            _emit(
                {
                    "jsonrpc": "2.0",
                    "id": None,
                    "error": {"code": -32700, "message": f"bridge parse error: {exc.msg}"},
                }
            )
            continue

        request_id = request_obj.get("id") if isinstance(request_obj, dict) else None
        try:
            response = _post_json(mcp_url, request_obj)
        except (urllib.error.URLError, TimeoutError, ValueError, json.JSONDecodeError) as exc:
            response = {
                "jsonrpc": "2.0",
                "id": request_id,
                "error": {"code": -32000, "message": f"bridge forward error: {exc}"},
            }

        _emit(response)

    return 0
