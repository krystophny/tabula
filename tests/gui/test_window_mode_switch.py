from __future__ import annotations

from pathlib import Path

import pytest

pyside = pytest.importorskip("PySide6")
from PySide6.QtWidgets import QApplication

from tabula.events import ClearCanvasEvent, TextArtifactEvent
from tabula.window import CanvasWindow
from tabula.watcher import PollResult


def test_window_mode_switches_prompt_discussion_prompt(tmp_path: Path) -> None:
    app = QApplication.instance() or QApplication([])
    window = CanvasWindow(tmp_path / "events.jsonl", poll_interval_ms=10_000)

    assert "prompt" in window.mode_label.text()

    window.apply_event(
        TextArtifactEvent(
            event_id="e1",
            ts="2026-02-11T12:00:00Z",
            kind="text_artifact",
            title="draft",
            text="hello",
        )
    )
    assert "discussion" in window.mode_label.text()

    window.apply_event(
        ClearCanvasEvent(
            event_id="e2",
            ts="2026-02-11T12:00:01Z",
            kind="clear_canvas",
            reason=None,
        )
    )
    assert "prompt" in window.mode_label.text()


def test_window_poll_once_uses_watcher_results(tmp_path: Path, monkeypatch) -> None:
    app = QApplication.instance() or QApplication([])
    window = CanvasWindow(tmp_path / "events.jsonl", poll_interval_ms=10_000)

    calls = {"count": 0}

    def fake_poll() -> PollResult:
        calls["count"] += 1
        return PollResult(
            events=[
                TextArtifactEvent(
                    event_id="e1",
                    ts="2026-02-11T12:00:00Z",
                    kind="text_artifact",
                    title="draft",
                    text="hello",
                )
            ],
            errors=["line 2: invalid JSON"],
        )

    monkeypatch.setattr(window, "_watcher", type("FakeWatcher", (), {"poll": staticmethod(fake_poll)})())
    window.poll_once()

    assert calls["count"] == 1
    assert "discussion" in window.mode_label.text()
    assert "line 2: invalid JSON" in window.status_label.text()
