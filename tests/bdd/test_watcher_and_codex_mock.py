from __future__ import annotations

import json
from dataclasses import dataclass
from pathlib import Path

import tabula.watcher as watcher_module
from tabula.events import ClearCanvasEvent
from tabula.state import CanvasState, reduce_state
from tabula.watcher import JsonlEventWatcher


@dataclass
class FakeCodexProducer:
    events_path: Path

    def emit(self, payload: dict[str, object]) -> None:
        self.events_path.parent.mkdir(parents=True, exist_ok=True)
        with self.events_path.open("a", encoding="utf-8") as handle:
            handle.write(json.dumps(payload) + "\n")


def test_given_codex_emits_events_when_canvas_polls_then_only_new_lines_are_processed(tmp_path: Path) -> None:
    events_path = tmp_path / "canvas-events.jsonl"
    producer = FakeCodexProducer(events_path)
    watcher = JsonlEventWatcher(events_path, base_dir=tmp_path)

    producer.emit(
        {
            "event_id": "e1",
            "ts": "2026-02-11T12:00:00Z",
            "kind": "text_artifact",
            "title": "draft",
            "text": "hello",
        }
    )
    first = watcher.poll()
    assert [event.event_id for event in first.events] == ["e1"]
    assert first.errors == []

    second = watcher.poll()
    assert second.events == []
    assert second.errors == []

    producer.emit(
        {
            "event_id": "e2",
            "ts": "2026-02-11T12:00:01Z",
            "kind": "clear_canvas",
        }
    )
    third = watcher.poll()
    assert [event.event_id for event in third.events] == ["e2"]
    assert third.errors == []


def test_given_malformed_stream_when_canvas_polls_then_parser_errors_are_kept_and_stream_continues(
    tmp_path: Path, monkeypatch
) -> None:
    events_path = tmp_path / "events.jsonl"
    events_path.write_text("ok-1\nbad\nok-2\n", encoding="utf-8")

    calls: list[str] = []

    def fake_parse_event_line(line: str, *, line_no: int | None = None, base_dir: Path | None = None):
        calls.append(line)
        if line == "bad":
            raise watcher_module.EventValidationError("line content broken", line_no=line_no)
        return ClearCanvasEvent(
            event_id=line,
            ts="2026-02-11T12:00:00Z",
            kind="clear_canvas",
            reason=None,
        )

    monkeypatch.setattr(watcher_module, "parse_event_line", fake_parse_event_line)

    watcher = JsonlEventWatcher(events_path, base_dir=tmp_path)
    result = watcher.poll()

    assert calls == ["ok-1", "bad", "ok-2"]
    assert [event.event_id for event in result.events] == ["ok-1", "ok-2"]
    assert len(result.errors) == 1
    assert "line 2: line content broken" in result.errors[0]


def test_given_full_mode_cycle_when_events_reduce_state_then_prompt_discussion_prompt(tmp_path: Path) -> None:
    events_path = tmp_path / "stream.jsonl"
    producer = FakeCodexProducer(events_path)
    watcher = JsonlEventWatcher(events_path, base_dir=tmp_path)
    state = CanvasState()

    producer.emit(
        {
            "event_id": "e1",
            "ts": "2026-02-11T12:00:00Z",
            "kind": "text_artifact",
            "title": "draft",
            "text": "v1",
        }
    )
    for event in watcher.poll().events:
        state = reduce_state(state, event)
    assert state.mode == "discussion"

    producer.emit(
        {
            "event_id": "e2",
            "ts": "2026-02-11T12:00:01Z",
            "kind": "clear_canvas",
        }
    )
    for event in watcher.poll().events:
        state = reduce_state(state, event)
    assert state.mode == "prompt"
