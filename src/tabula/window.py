from __future__ import annotations

from pathlib import Path

from .events import CanvasEvent
from .state import CanvasState, reduce_state
from .watcher import JsonlEventWatcher

from PySide6.QtCore import Qt, QTimer
from PySide6.QtGui import QPixmap
from PySide6.QtWidgets import (
    QApplication,
    QLabel,
    QMainWindow,
    QPlainTextEdit,
    QStackedWidget,
    QVBoxLayout,
    QWidget,
)

try:
    from PySide6.QtPdf import QPdfDocument
    from PySide6.QtPdfWidgets import QPdfView

    HAS_QTPDF = True
except Exception:  # pragma: no cover
    QPdfDocument = None
    QPdfView = None
    HAS_QTPDF = False


class CanvasWindow(QMainWindow):
    def __init__(self, events_path: Path, *, poll_interval_ms: int = 250) -> None:
        super().__init__()
        self.setWindowTitle("Tabula Canvas")
        self.resize(1000, 700)

        self._state = CanvasState()
        self._watcher = JsonlEventWatcher(events_path)

        root = QWidget(self)
        layout = QVBoxLayout(root)

        self.mode_label = QLabel("mode: prompt")
        self.mode_label.setObjectName("modeLabel")
        layout.addWidget(self.mode_label)

        self.status_label = QLabel("status: waiting for events")
        self.status_label.setObjectName("statusLabel")
        layout.addWidget(self.status_label)

        self.stack = QStackedWidget()
        layout.addWidget(self.stack, 1)

        self.blank_label = QLabel("Canvas inactive")
        self.blank_label.setAlignment(Qt.AlignmentFlag.AlignCenter)
        self.stack.addWidget(self.blank_label)

        self.text_view = QPlainTextEdit()
        self.text_view.setReadOnly(True)
        self.stack.addWidget(self.text_view)

        self.image_label = QLabel("image")
        self.image_label.setAlignment(Qt.AlignmentFlag.AlignCenter)
        self.image_label.setScaledContents(False)
        self.stack.addWidget(self.image_label)

        if HAS_QTPDF:
            self.pdf_document = QPdfDocument(self)
            self.pdf_view = QPdfView()
            self.pdf_view.setDocument(self.pdf_document)
            self.stack.addWidget(self.pdf_view)
        else:
            self.pdf_document = None
            self.pdf_view = QLabel("QtPdf unavailable")
            self.pdf_view.setAlignment(Qt.AlignmentFlag.AlignCenter)
            self.stack.addWidget(self.pdf_view)

        self.setCentralWidget(root)

        self._timer = QTimer(self)
        self._timer.timeout.connect(self.poll_once)
        self._timer.start(poll_interval_ms)

    def apply_event(self, event: CanvasEvent) -> None:
        self._state = reduce_state(self._state, event)
        self.mode_label.setText(f"mode: {self._state.mode}")

        if event.kind == "clear_canvas":
            self.stack.setCurrentWidget(self.blank_label)
            self.status_label.setText("status: canvas cleared")
            return

        if event.kind == "text_artifact":
            self.text_view.setPlainText(event.text)
            self.stack.setCurrentWidget(self.text_view)
            self.status_label.setText(f"status: text artifact '{event.title}'")
            return

        if event.kind == "image_artifact":
            pixmap = QPixmap(event.path)
            if pixmap.isNull():
                self.status_label.setText(f"status: failed to load image {event.path}")
                return
            self.image_label.setPixmap(pixmap)
            self.stack.setCurrentWidget(self.image_label)
            self.status_label.setText(f"status: image artifact '{event.title}'")
            return

        if event.kind == "pdf_artifact":
            if HAS_QTPDF and self.pdf_document is not None:
                self.pdf_document.load(event.path)
                if self.pdf_document.status() == QPdfDocument.Status.Ready:
                    if hasattr(self.pdf_view, "setPageMode"):
                        pass
                    self.stack.setCurrentWidget(self.pdf_view)
                    self.status_label.setText(f"status: pdf artifact '{event.title}'")
                else:
                    self.status_label.setText(f"status: failed to load pdf {event.path}")
            else:
                self.stack.setCurrentWidget(self.pdf_view)
                self.status_label.setText("status: QtPdf unavailable")

    def poll_once(self) -> None:
        result = self._watcher.poll()
        for event in result.events:
            self.apply_event(event)
        if result.errors:
            self.status_label.setText("status: " + result.errors[-1])


def run_canvas(events_path: Path, *, poll_interval_ms: int = 250) -> int:
    app = QApplication.instance() or QApplication([])
    window = CanvasWindow(events_path, poll_interval_ms=poll_interval_ms)
    window.show()
    return app.exec()
