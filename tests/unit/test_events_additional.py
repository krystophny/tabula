from __future__ import annotations

import json
from pathlib import Path

import pytest

from tabula.events import EventValidationError, parse_event_line


def test_parse_empty_line_rejected() -> None:
    with pytest.raises(EventValidationError):
        parse_event_line("   ")


def test_parse_non_object_payload_rejected() -> None:
    with pytest.raises(EventValidationError):
        parse_event_line("[]")


def test_parse_invalid_timestamp_rejected() -> None:
    line = json.dumps(
        {
            "event_id": "e1",
            "ts": "not-a-timestamp",
            "kind": "text_artifact",
            "title": "t",
            "text": "x",
        }
    )
    with pytest.raises(EventValidationError):
        parse_event_line(line)


def test_parse_blank_event_id_rejected() -> None:
    line = json.dumps(
        {
            "event_id": "   ",
            "ts": "2026-02-11T12:00:00Z",
            "kind": "text_artifact",
            "title": "t",
            "text": "x",
        }
    )
    with pytest.raises(EventValidationError):
        parse_event_line(line)


def test_text_artifact_extra_field_rejected() -> None:
    line = json.dumps(
        {
            "event_id": "e1",
            "ts": "2026-02-11T12:00:00Z",
            "kind": "text_artifact",
            "title": "t",
            "text": "x",
            "extra": "bad",
        }
    )
    with pytest.raises(EventValidationError):
        parse_event_line(line)


def test_text_artifact_non_string_text_rejected() -> None:
    line = json.dumps(
        {
            "event_id": "e1",
            "ts": "2026-02-11T12:00:00Z",
            "kind": "text_artifact",
            "title": "t",
            "text": 1,
        }
    )
    with pytest.raises(EventValidationError):
        parse_event_line(line)


def test_image_artifact_extra_field_rejected(tmp_path: Path) -> None:
    image = tmp_path / "img.png"
    image.write_bytes(b"x")
    line = json.dumps(
        {
            "event_id": "e2",
            "ts": "2026-02-11T12:00:00Z",
            "kind": "image_artifact",
            "title": "img",
            "path": str(image),
            "extra": True,
        }
    )
    with pytest.raises(EventValidationError):
        parse_event_line(line, base_dir=tmp_path)


def test_pdf_artifact_extra_field_rejected(tmp_path: Path) -> None:
    doc = tmp_path / "doc.pdf"
    doc.write_bytes(b"%PDF-1.4")
    line = json.dumps(
        {
            "event_id": "e3",
            "ts": "2026-02-11T12:00:00Z",
            "kind": "pdf_artifact",
            "title": "doc",
            "path": str(doc),
            "page": 0,
            "extra": "bad",
        }
    )
    with pytest.raises(EventValidationError):
        parse_event_line(line, base_dir=tmp_path)


def test_pdf_artifact_missing_required_field_rejected(tmp_path: Path) -> None:
    line = json.dumps(
        {
            "event_id": "e3",
            "ts": "2026-02-11T12:00:00Z",
            "kind": "pdf_artifact",
            "title": "doc",
        }
    )
    with pytest.raises(EventValidationError):
        parse_event_line(line, base_dir=tmp_path)


def test_clear_canvas_extra_field_rejected() -> None:
    line = json.dumps(
        {
            "event_id": "e4",
            "ts": "2026-02-11T12:00:00Z",
            "kind": "clear_canvas",
            "extra": "bad",
        }
    )
    with pytest.raises(EventValidationError):
        parse_event_line(line)


def test_unsupported_kind_rejected() -> None:
    line = json.dumps(
        {
            "event_id": "e9",
            "ts": "2026-02-11T12:00:00Z",
            "kind": "audio_artifact",
        }
    )
    with pytest.raises(EventValidationError):
        parse_event_line(line)
