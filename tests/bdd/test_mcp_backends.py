from __future__ import annotations

import pytest

from tabula.mcp_backends import build_http_backends, parse_backend_specs


def test_parse_backend_specs_parses_name_url_pairs() -> None:
    parsed = parse_backend_specs([
        "helpy=http://127.0.0.1:8090/mcp",
        "mail=http://localhost:8091/mcp",
    ])
    assert parsed == {
        "helpy": "http://127.0.0.1:8090/mcp",
        "mail": "http://localhost:8091/mcp",
    }


@pytest.mark.parametrize(
    "specs",
    [
        ["invalid"],
        ["=http://127.0.0.1:1/mcp"],
        ["name="],
        ["dupe=http://127.0.0.1:1/mcp", "dupe=http://127.0.0.1:2/mcp"],
    ],
)
def test_parse_backend_specs_rejects_invalid_inputs(specs: list[str]) -> None:
    with pytest.raises(ValueError):
        parse_backend_specs(specs)


def test_build_http_backends_empty_returns_empty() -> None:
    assert build_http_backends(None) == {}
    assert build_http_backends({}) == {}


def test_build_http_backends_constructs_named_backends() -> None:
    backends = build_http_backends({"helpy": "http://127.0.0.1:8090/mcp"})
    assert "helpy" in backends
    assert backends["helpy"].name == "helpy"
    assert backends["helpy"].url == "http://127.0.0.1:8090/mcp"
