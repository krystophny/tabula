from __future__ import annotations

import json
from pathlib import Path

from tabula.state import CanvasState, reduce_state
from tabula.watcher import JsonlEventWatcher


def test_watcher_reads_appended_events_and_skips_invalid(tmp_path: Path) -> None:
    events_path = tmp_path / "canvas-events.jsonl"
    watcher = JsonlEventWatcher(events_path, base_dir=tmp_path)

    image = tmp_path / "img.png"
    image.write_bytes(b"x")

    events_path.write_text(
        "\n".join(
            [
                json.dumps(
                    {
                        "event_id": "e1",
                        "ts": "2026-02-11T12:00:00Z",
                        "kind": "text_artifact",
                        "title": "t",
                        "text": "a",
                    }
                ),
                "{bad json",
                json.dumps(
                    {
                        "event_id": "e2",
                        "ts": "2026-02-11T12:00:01Z",
                        "kind": "image_artifact",
                        "title": "img",
                        "path": str(image),
                    }
                ),
                json.dumps(
                    {
                        "event_id": "e3",
                        "ts": "2026-02-11T12:00:02Z",
                        "kind": "clear_canvas",
                    }
                ),
            ]
        ),
        encoding="utf-8",
    )

    result = watcher.poll()
    assert [event.kind for event in result.events] == ["text_artifact", "image_artifact", "clear_canvas"]
    assert len(result.errors) == 1

    state = CanvasState()
    for event in result.events:
        state = reduce_state(state, event)
    assert state.mode == "prompt"
