from __future__ import annotations

from tabula.events import ClearCanvasEvent, TextArtifactEvent
from tabula.state import CanvasState, reduce_state


def test_prompt_to_review_on_artifact() -> None:
    state = CanvasState(mode="prompt", active_event=None)
    event = TextArtifactEvent(
        event_id="e1",
        ts="2026-02-11T12:00:00Z",
        kind="text_artifact",
        title="hello",
        text="world",
    )
    next_state = reduce_state(state, event)
    assert next_state.mode == "review"
    assert next_state.active_event == event


def test_review_to_prompt_on_clear() -> None:
    state = CanvasState(mode="review", active_event=None)
    event = ClearCanvasEvent(
        event_id="e2",
        ts="2026-02-11T12:00:01Z",
        kind="clear_canvas",
        reason="done",
    )
    next_state = reduce_state(state, event)
    assert next_state.mode == "prompt"
    assert next_state.active_event is None
