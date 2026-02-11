from __future__ import annotations

import json
from pathlib import Path

import pytest

from tabula.events import ClearCanvasEvent, EventValidationError, parse_event_line
from tabula.state import CanvasState, reduce_state


@pytest.mark.parametrize(
    ("payload", "expected_kind"),
    [
        (
            {
                "event_id": "e1",
                "ts": "2026-02-11T12:00:00Z",
                "kind": "text_artifact",
                "title": "draft",
                "text": "hello",
            },
            "text_artifact",
        ),
        (
            {
                "event_id": "e2",
                "ts": "2026-02-11T12:00:01Z",
                "kind": "image_artifact",
                "title": "image",
                "path": "img.png",
            },
            "image_artifact",
        ),
        (
            {
                "event_id": "e3",
                "ts": "2026-02-11T12:00:02Z",
                "kind": "pdf_artifact",
                "title": "paper",
                "path": "doc.pdf",
                "page": 0,
            },
            "pdf_artifact",
        ),
    ],
)
def test_given_prompt_mode_when_artifact_event_arrives_then_mode_switches_to_review(
    tmp_path: Path, payload: dict[str, object], expected_kind: str
) -> None:
    if payload["kind"] == "image_artifact":
        (tmp_path / "img.png").write_bytes(b"x")
    if payload["kind"] == "pdf_artifact":
        (tmp_path / "doc.pdf").write_bytes(b"%PDF-1.4")

    line = json.dumps(payload)
    event = parse_event_line(line, base_dir=tmp_path)
    next_state = reduce_state(CanvasState(mode="prompt", active_event=None), event)

    assert event.kind == expected_kind
    assert next_state.mode == "review"
    assert next_state.active_event == event


def test_given_review_mode_when_clear_canvas_arrives_then_mode_switches_back_to_prompt() -> None:
    current = CanvasState(mode="review", active_event=None)
    clear = ClearCanvasEvent(
        event_id="clear-1",
        ts="2026-02-11T12:00:03Z",
        kind="clear_canvas",
        reason="done",
    )

    next_state = reduce_state(current, clear)
    assert next_state.mode == "prompt"
    assert next_state.active_event is None


@pytest.mark.parametrize(
    "line",
    [
        '{"event_id":"e1","ts":"2026-02-11T12:00:00Z","kind":"text_artifact","title":"x","text":"y","extra":true}',
        '{"event_id":"e2","ts":"2026-02-11T12:00:00Z","kind":"image_artifact","title":"x","path":"img.png","extra":"bad"}',
        '{"event_id":"e3","ts":"2026-02-11T12:00:00Z","kind":"pdf_artifact","title":"x","path":"doc.pdf","page":-1}',
        '{"event_id":"e4","ts":"2026-02-11T12:00:00Z","kind":"clear_canvas","reason":42}',
    ],
)
def test_given_invalid_event_payload_when_parsed_then_strict_validation_rejects(tmp_path: Path, line: str) -> None:
    (tmp_path / "img.png").write_bytes(b"x")
    (tmp_path / "doc.pdf").write_bytes(b"%PDF-1.4")

    with pytest.raises(EventValidationError):
        parse_event_line(line, base_dir=tmp_path)
