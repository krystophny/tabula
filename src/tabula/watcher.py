from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path

from .events import CanvasEvent, EventValidationError, parse_event_line


@dataclass(frozen=True)
class PollResult:
    events: list[CanvasEvent]
    errors: list[str]


class JsonlEventWatcher:
    def __init__(self, events_path: Path, *, base_dir: Path | None = None) -> None:
        self._events_path = events_path
        self._base_dir = base_dir or events_path.parent
        self._offset = 0
        self._line_no = 0

    def poll(self) -> PollResult:
        self._events_path.parent.mkdir(parents=True, exist_ok=True)
        self._events_path.touch(exist_ok=True)

        events: list[CanvasEvent] = []
        errors: list[str] = []

        with self._events_path.open("r", encoding="utf-8") as handle:
            handle.seek(self._offset)
            while True:
                raw = handle.readline()
                if not raw:
                    break
                self._line_no += 1
                line = raw.strip()
                if not line:
                    continue
                try:
                    event = parse_event_line(line, line_no=self._line_no, base_dir=self._base_dir)
                except EventValidationError as exc:
                    errors.append(str(exc))
                    continue
                events.append(event)

            self._offset = handle.tell()

        return PollResult(events=events, errors=errors)
