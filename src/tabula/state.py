from __future__ import annotations

from dataclasses import dataclass
from typing import Literal

from .events import CanvasEvent

Mode = Literal["prompt", "review"]


@dataclass(frozen=True)
class CanvasState:
    mode: Mode = "prompt"
    active_event: CanvasEvent | None = None


def reduce_state(state: CanvasState, event: CanvasEvent) -> CanvasState:
    if event.kind == "clear_canvas":
        return CanvasState(mode="prompt", active_event=None)
    return CanvasState(mode="review", active_event=event)
