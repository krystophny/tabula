from __future__ import annotations

import json
import os
import signal
import subprocess
import sys
import threading
import time
from dataclasses import dataclass, field
from datetime import datetime, timezone
from pathlib import Path
from typing import Callable, Mapping
from uuid import uuid4

from .events import CanvasEvent, event_to_payload, parse_event_payload
from .state import CanvasState, reduce_state


def has_display(env: Mapping[str, str] | None = None) -> bool:
    source = env or os.environ
    return bool(source.get("DISPLAY") or source.get("WAYLAND_DISPLAY"))


def launch_canvas_background(project_dir: Path, *, poll_interval_ms: int = 250) -> subprocess.Popen[bytes]:
    cmd = [
        sys.executable,
        "-m",
        "tabula",
        "canvas",
        "--poll-ms",
        str(poll_interval_ms),
    ]
    return subprocess.Popen(
        cmd,
        cwd=project_dir,
        stdin=subprocess.PIPE,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )


@dataclass
class TextSelection:
    event_id: str
    line_start: int
    line_end: int
    text: str


@dataclass
class SessionRecord:
    state: CanvasState
    activated: bool
    history: list[CanvasEvent] = field(default_factory=list)
    selection: TextSelection | None = None


class CanvasAdapter:
    def __init__(
        self,
        *,
        project_dir: Path,
        start_canvas: bool = True,
        headless: bool = False,
        fresh_canvas: bool = False,
        poll_interval_ms: int = 250,
        env: Mapping[str, str] | None = None,
        on_event: Callable[[CanvasEvent], None] | None = None,
    ) -> None:
        self._project_dir = project_dir.resolve()
        self._start_canvas = start_canvas
        self._headless_override = headless
        self._fresh_canvas = fresh_canvas
        self._poll_interval_ms = poll_interval_ms
        self._env = env
        self._on_event = on_event

        self._lock = threading.RLock()
        self._sessions: dict[str, SessionRecord] = {}
        self._event_to_session: dict[str, str] = {}
        self._canvas_proc: subprocess.Popen[bytes] | None = None
        self._canvas_feedback_thread: threading.Thread | None = None
        self._canvas_feedback_pid: int | None = None
        self._canvas_launch_error: str | None = None

    def _effective_headless(self) -> bool:
        return self._headless_override or not has_display(self._env)

    def _canvas_pid_path(self) -> Path:
        return self._project_dir / ".tabula" / "canvas.pid"

    @staticmethod
    def _is_tabula_canvas_pid(pid: int) -> bool:
        cmdline_path = Path("/proc") / str(pid) / "cmdline"
        try:
            raw = cmdline_path.read_bytes()
        except OSError:
            return False
        text = raw.decode("utf-8", errors="ignore")
        return ("tabula" in text) and ("canvas" in text)

    def _clear_canvas_pid_file(self) -> None:
        try:
            self._canvas_pid_path().unlink()
        except FileNotFoundError:
            return

    def _write_canvas_pid_file(self, pid: int) -> None:
        path = self._canvas_pid_path()
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(str(pid), encoding="utf-8")

    def _terminate_stale_canvas_from_pid_file(self) -> None:
        path = self._canvas_pid_path()
        if not path.exists():
            return

        try:
            pid = int(path.read_text(encoding="utf-8").strip())
        except ValueError:
            self._clear_canvas_pid_file()
            return

        if pid <= 0 or not self._is_tabula_canvas_pid(pid):
            self._clear_canvas_pid_file()
            return

        try:
            os.kill(pid, signal.SIGTERM)
        except ProcessLookupError:
            self._clear_canvas_pid_file()
            return
        except PermissionError:
            return

        deadline = time.time() + 1.0
        while time.time() < deadline:
            try:
                os.kill(pid, 0)
            except ProcessLookupError:
                self._clear_canvas_pid_file()
                return
            except PermissionError:
                return
            time.sleep(0.05)

        try:
            os.kill(pid, signal.SIGKILL)
        except (ProcessLookupError, PermissionError):
            pass
        self._clear_canvas_pid_file()

    def _ensure_canvas_process(self) -> None:
        if self._effective_headless() or not self._start_canvas:
            self._canvas_launch_error = None
            return
        if self._canvas_proc is not None and self._canvas_proc.poll() is None:
            return
        if self._fresh_canvas:
            self._terminate_stale_canvas_from_pid_file()
        self._canvas_proc = launch_canvas_background(self._project_dir, poll_interval_ms=self._poll_interval_ms)
        if self._canvas_proc.pid > 0:
            self._write_canvas_pid_file(self._canvas_proc.pid)

        # Detect immediate startup failures (for example missing PySide6/Qt deps).
        # Without this, callers may see headless=false even though no window exists.
        for _ in range(5):
            if self._canvas_proc.poll() is not None:
                break
            time.sleep(0.05)
        if self._canvas_proc.poll() is None:
            self._canvas_launch_error = None
            self._start_canvas_feedback_reader()
            return

        exit_code = self._canvas_proc.poll()
        err_text = ""
        try:
            if self._canvas_proc.stderr is not None:
                raw_err = self._canvas_proc.stderr.read() or b""
                err_text = raw_err.decode("utf-8", errors="replace").strip()
        except OSError:
            err_text = ""

        detail = f"canvas process exited early with code {exit_code}"
        if err_text:
            detail += f": {err_text.splitlines()[-1]}"
        self._canvas_launch_error = detail
        self._canvas_proc = None
        self._clear_canvas_pid_file()

    def _canvas_process_alive(self) -> bool:
        return self._canvas_proc is not None and self._canvas_proc.poll() is None

    def _start_canvas_feedback_reader(self) -> None:
        proc = self._canvas_proc
        if proc is None or proc.stdout is None:
            return
        if self._canvas_feedback_pid == proc.pid and self._canvas_feedback_thread is not None:
            if self._canvas_feedback_thread.is_alive():
                return

        self._canvas_feedback_pid = proc.pid
        self._canvas_feedback_thread = threading.Thread(
            target=self._canvas_feedback_reader_loop,
            args=(proc,),
            daemon=True,
        )
        self._canvas_feedback_thread.start()

    def _canvas_feedback_reader_loop(self, proc: subprocess.Popen[bytes]) -> None:
        if proc.stdout is None:
            return
        while True:
            try:
                raw = proc.stdout.readline()
            except OSError:
                return
            if not raw:
                return
            line = raw.decode("utf-8", errors="replace").strip()
            if not line:
                continue
            self._handle_canvas_feedback_line(line)

    def _handle_canvas_feedback_line(self, line: str) -> None:
        try:
            payload = json.loads(line)
        except json.JSONDecodeError:
            return
        if not isinstance(payload, dict):
            return
        if payload.get("kind") != "text_selection":
            return

        event_id = payload.get("event_id")
        if not isinstance(event_id, str) or not event_id.strip():
            return

        line_start = payload.get("line_start")
        line_end = payload.get("line_end")
        text = payload.get("text")

        if line_start is None or line_end is None or text is None:
            self._apply_text_selection_feedback(event_id=event_id, line_start=None, line_end=None, text=None)
            return

        if not isinstance(line_start, int) or line_start < 1:
            return
        if not isinstance(line_end, int) or line_end < line_start:
            return
        if not isinstance(text, str):
            return

        self._apply_text_selection_feedback(event_id=event_id, line_start=line_start, line_end=line_end, text=text)

    def _apply_text_selection_feedback(
        self,
        *,
        event_id: str,
        line_start: int | None,
        line_end: int | None,
        text: str | None,
    ) -> None:
        with self._lock:
            session_id = self._event_to_session.get(event_id)
            if session_id is None:
                return

            record = self._ensure_session(session_id)
            active = record.state.active_event
            if active is None or active.kind != "text_artifact" or active.event_id != event_id:
                return

            if line_start is None or line_end is None or text is None or text == "":
                record.selection = None
                return
            record.selection = TextSelection(
                event_id=event_id,
                line_start=line_start,
                line_end=line_end,
                text=text,
            )

    def handle_feedback(self, line: str) -> None:
        self._handle_canvas_feedback_line(line)

    def _ensure_session(self, session_id: str) -> SessionRecord:
        if session_id not in self._sessions:
            self._sessions[session_id] = SessionRecord(state=CanvasState(), activated=False)
        return self._sessions[session_id]

    def _prune_stale_event_mappings(self, session_id: str, record: SessionRecord) -> None:
        live_ids = {ev.event_id for ev in record.history}
        stale = [eid for eid, sid in self._event_to_session.items() if sid == session_id and eid not in live_ids]
        for eid in stale:
            del self._event_to_session[eid]

    def list_sessions(self) -> list[str]:
        with self._lock:
            return sorted(self._sessions.keys())

    @staticmethod
    def _base_payload(kind: str) -> dict[str, object]:
        return {
            "event_id": str(uuid4()),
            "ts": datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
            "kind": kind,
        }

    def _emit_to_canvas(self, event: CanvasEvent) -> None:
        if self._on_event is not None:
            self._on_event(event)
        self._ensure_canvas_process()
        proc = self._canvas_proc
        if proc is None or proc.stdin is None:
            return

        try:
            line = json.dumps(event_to_payload(event), separators=(",", ":")) + "\n"
            proc.stdin.write(line.encode("utf-8"))
            proc.stdin.flush()
        except (BrokenPipeError, OSError):
            self._canvas_proc = None
            self._clear_canvas_pid_file()

    def _record_event(self, session_id: str, event: CanvasEvent) -> SessionRecord:
        with self._lock:
            record = self._ensure_session(session_id)
            record.state = reduce_state(record.state, event)
            record.selection = None
            record.history.append(event)
            self._event_to_session[event.event_id] = session_id
            if event.kind == "clear_canvas":
                self._prune_stale_event_mappings(session_id, record)
        self._emit_to_canvas(event)
        return record

    @staticmethod
    def _selection_payload(record: SessionRecord) -> dict[str, object]:
        selection = record.selection
        if selection is None:
            return {
                "has_selection": False,
                "event_id": None,
                "line_start": None,
                "line_end": None,
                "text": None,
            }
        return {
            "has_selection": True,
            "event_id": selection.event_id,
            "line_start": selection.line_start,
            "line_end": selection.line_end,
            "text": selection.text,
        }

    def canvas_activate(self, *, session_id: str, mode_hint: str | None = None) -> dict[str, object]:
        if not session_id.strip():
            raise ValueError("session_id must be non-empty")
        if mode_hint == "discussion":
            mode_hint = "review"
        with self._lock:
            record = self._ensure_session(session_id)
            record.activated = True
            self._ensure_canvas_process()
            return {
                "active": True,
                "headless": self._effective_headless(),
                "mode": record.state.mode,
                "mode_hint": mode_hint,
                "selection": self._selection_payload(record),
                "canvas_process_alive": self._canvas_process_alive(),
                "canvas_launch_error": self._canvas_launch_error,
            }

    def canvas_render_text(self, *, session_id: str, title: str, markdown_or_text: str) -> dict[str, object]:
        if not title.strip():
            raise ValueError("title must be non-empty")
        if not isinstance(markdown_or_text, str):
            raise ValueError("markdown_or_text must be a string")

        with self._lock:
            self.canvas_activate(session_id=session_id)
            payload = self._base_payload("text_artifact")
            payload.update({"title": title, "text": markdown_or_text})
            event = parse_event_payload(payload, base_dir=self._project_dir)

            record = self._record_event(session_id, event)
            return {
                "artifact_id": event.event_id,
                "kind": "text_artifact",
                "mode": record.state.mode,
            }

    def canvas_render_image(self, *, session_id: str, title: str, path: str) -> dict[str, object]:
        if not title.strip():
            raise ValueError("title must be non-empty")
        if not isinstance(path, str) or not path.strip():
            raise ValueError("path must be a non-empty string")

        with self._lock:
            self.canvas_activate(session_id=session_id)
            payload = self._base_payload("image_artifact")
            payload.update({"title": title, "path": path})
            event = parse_event_payload(payload, base_dir=self._project_dir)

            record = self._record_event(session_id, event)
            return {
                "artifact_id": event.event_id,
                "kind": "image_artifact",
                "path": event.path,
                "mode": record.state.mode,
            }

    def canvas_render_pdf(self, *, session_id: str, title: str, path: str, page: int = 0) -> dict[str, object]:
        if not title.strip():
            raise ValueError("title must be non-empty")
        if not isinstance(path, str) or not path.strip():
            raise ValueError("path must be a non-empty string")
        if not isinstance(page, int) or page < 0:
            raise ValueError("page must be integer >= 0")

        with self._lock:
            self.canvas_activate(session_id=session_id)
            payload = self._base_payload("pdf_artifact")
            payload.update({"title": title, "path": path, "page": page})
            event = parse_event_payload(payload, base_dir=self._project_dir)

            record = self._record_event(session_id, event)
            return {
                "artifact_id": event.event_id,
                "kind": "pdf_artifact",
                "path": event.path,
                "page": event.page,
                "mode": record.state.mode,
            }

    def canvas_clear(self, *, session_id: str, reason: str | None = None) -> dict[str, object]:
        with self._lock:
            self.canvas_activate(session_id=session_id)
            payload = self._base_payload("clear_canvas")
            if reason is not None:
                payload["reason"] = reason
            event = parse_event_payload(payload, base_dir=self._project_dir)

            record = self._record_event(session_id, event)
            return {"cleared": True, "mode": record.state.mode}

    def canvas_status(self, *, session_id: str) -> dict[str, object]:
        with self._lock:
            record = self._ensure_session(session_id)
            active_event = record.state.active_event
            event_id = active_event.event_id if active_event is not None else None
            kind = active_event.kind if active_event is not None else None
            return {
                "mode": record.state.mode,
                "active": record.activated,
                "active_event_id": event_id,
                "active_kind": kind,
                "history_size": len(record.history),
                "headless": self._effective_headless(),
                "selection": self._selection_payload(record),
                "canvas_process_alive": self._canvas_process_alive(),
                "canvas_launch_error": self._canvas_launch_error,
            }

    def canvas_selection(self, *, session_id: str) -> dict[str, object]:
        with self._lock:
            record = self._ensure_session(session_id)
            active_event = record.state.active_event
            event_id = active_event.event_id if active_event is not None else None
            return {
                "session_id": session_id,
                "mode": record.state.mode,
                "active_event_id": event_id,
                "selection": self._selection_payload(record),
            }

    def canvas_history(self, *, session_id: str, limit: int = 20) -> dict[str, object]:
        if not isinstance(limit, int) or limit <= 0:
            raise ValueError("limit must be integer > 0")

        with self._lock:
            record = self._ensure_session(session_id)
            selected = record.history[-limit:]
            return {
                "session_id": session_id,
                "count": len(selected),
                "events": [event_to_payload(event) for event in selected],
            }
