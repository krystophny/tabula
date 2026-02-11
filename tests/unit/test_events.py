from __future__ import annotations

import json
from pathlib import Path

import pytest

from tabula.events import EventValidationError, event_schema, parse_event_line


def test_parse_text_artifact_valid() -> None:
    line = json.dumps(
        {
            "event_id": "e1",
            "ts": "2026-02-11T12:00:00Z",
            "kind": "text_artifact",
            "title": "draft",
            "text": "hello",
        }
    )
    event = parse_event_line(line)
    assert event.kind == "text_artifact"
    assert event.text == "hello"


def test_parse_text_artifact_missing_field() -> None:
    line = json.dumps(
        {
            "event_id": "e1",
            "ts": "2026-02-11T12:00:00Z",
            "kind": "text_artifact",
            "title": "draft",
        }
    )
    with pytest.raises(EventValidationError):
        parse_event_line(line)


def test_parse_image_path_must_be_local_existing(tmp_path: Path) -> None:
    image = tmp_path / "img.png"
    image.write_bytes(b"x")
    line = json.dumps(
        {
            "event_id": "e2",
            "ts": "2026-02-11T12:00:00+00:00",
            "kind": "image_artifact",
            "title": "img",
            "path": str(image),
        }
    )
    event = parse_event_line(line, base_dir=tmp_path)
    assert event.kind == "image_artifact"


def test_parse_image_url_rejected() -> None:
    line = json.dumps(
        {
            "event_id": "e2",
            "ts": "2026-02-11T12:00:00+00:00",
            "kind": "image_artifact",
            "title": "img",
            "path": "https://example.com/x.png",
        }
    )
    with pytest.raises(EventValidationError):
        parse_event_line(line)


def test_event_schema_contains_variants() -> None:
    schema = event_schema()
    assert schema["title"] == "TabulaCanvasEvent"
    assert len(schema["oneOf"]) == 4
