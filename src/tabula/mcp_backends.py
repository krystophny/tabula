from __future__ import annotations

import json
import urllib.error
import urllib.request
from dataclasses import dataclass
from typing import Any


class BackendError(RuntimeError):
    pass


@dataclass(frozen=True)
class HttpMcpBackend:
    name: str
    url: str
    timeout_s: float = 10.0

    def _rpc(self, method: str, params: dict[str, Any]) -> dict[str, Any]:
        body = json.dumps(
            {
                "jsonrpc": "2.0",
                "id": 1,
                "method": method,
                "params": params,
            },
            separators=(",", ":"),
        ).encode("utf-8")
        req = urllib.request.Request(
            self.url,
            data=body,
            headers={"Content-Type": "application/json"},
            method="POST",
        )
        try:
            with urllib.request.urlopen(req, timeout=self.timeout_s) as resp:  # noqa: S310
                payload = json.loads(resp.read().decode("utf-8"))
        except urllib.error.URLError as exc:  # pragma: no cover - network errors are environment-dependent
            raise BackendError(f"backend '{self.name}' unreachable: {exc.reason}") from exc
        except json.JSONDecodeError as exc:
            raise BackendError(f"backend '{self.name}' returned invalid JSON: {exc}") from exc

        if not isinstance(payload, dict):
            raise BackendError(f"backend '{self.name}' returned non-object response")

        error = payload.get("error")
        if isinstance(error, dict):
            message = error.get("message", "unknown error")
            raise BackendError(f"backend '{self.name}' RPC error: {message}")

        result = payload.get("result")
        if not isinstance(result, dict):
            raise BackendError(f"backend '{self.name}' returned invalid result payload")
        return result

    def list_tools(self) -> list[dict[str, Any]]:
        result = self._rpc("tools/list", {})
        tools = result.get("tools")
        if not isinstance(tools, list):
            raise BackendError(f"backend '{self.name}' returned invalid tools list")

        valid_tools: list[dict[str, Any]] = []
        for tool in tools:
            if isinstance(tool, dict):
                valid_tools.append(tool)
        return valid_tools

    def call_tool(self, name: str, arguments: dict[str, Any]) -> dict[str, Any]:
        result = self._rpc(
            "tools/call",
            {
                "name": name,
                "arguments": arguments,
            },
        )
        if not isinstance(result, dict):
            raise BackendError(f"backend '{self.name}' returned invalid tool result")
        return result


def parse_backend_specs(specs: list[str]) -> dict[str, str]:
    parsed: dict[str, str] = {}
    for raw in specs:
        item = raw.strip()
        if not item:
            continue
        if "=" not in item:
            raise ValueError(f"invalid backend spec '{raw}' (expected NAME=URL)")
        name, url = item.split("=", 1)
        name = name.strip()
        url = url.strip()
        if not name:
            raise ValueError(f"invalid backend spec '{raw}' (missing name)")
        if not url:
            raise ValueError(f"invalid backend spec '{raw}' (missing URL)")
        if name in parsed:
            raise ValueError(f"duplicate backend name '{name}'")
        parsed[name] = url
    return parsed


def build_http_backends(backend_urls: dict[str, str] | None) -> dict[str, HttpMcpBackend]:
    if not backend_urls:
        return {}
    return {
        name: HttpMcpBackend(name=name, url=url)
        for name, url in backend_urls.items()
    }
