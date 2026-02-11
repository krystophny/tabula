from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

from .events import EventValidationError, event_schema, parse_event_line


def _build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="tabula")
    sub = parser.add_subparsers(dest="command", required=True)

    p_canvas = sub.add_parser("canvas", help="launch canvas window")
    p_canvas.add_argument("--events", type=Path, default=Path(".tabula/canvas-events.jsonl"))
    p_canvas.add_argument("--poll-ms", type=int, default=250)

    p_check = sub.add_parser("check-events", help="validate JSONL event file")
    p_check.add_argument("--events", type=Path, required=True)

    sub.add_parser("schema", help="print JSON schema")
    return parser


def _cmd_canvas(events: Path, poll_ms: int) -> int:
    try:
        from .window import run_canvas
    except ModuleNotFoundError as exc:
        print(
            "PySide6 is required for 'tabula canvas'. Install with: python -m pip install -e .[gui]",
            file=sys.stderr,
        )
        return 2

    return run_canvas(events, poll_interval_ms=poll_ms)


def _cmd_check_events(events: Path) -> int:
    if not events.exists():
        print(f"event file does not exist: {events}", file=sys.stderr)
        return 1

    errors: list[str] = []
    for line_no, raw in enumerate(events.read_text(encoding="utf-8").splitlines(), start=1):
        if not raw.strip():
            continue
        try:
            parse_event_line(raw, line_no=line_no, base_dir=events.parent)
        except EventValidationError as exc:
            errors.append(str(exc))

    if errors:
        print("event validation failed:", file=sys.stderr)
        for err in errors:
            print(f"- {err}", file=sys.stderr)
        return 1

    print("event validation passed")
    return 0


def _cmd_schema() -> int:
    print(json.dumps(event_schema(), indent=2, sort_keys=True))
    return 0


def main(argv: list[str] | None = None) -> int:
    parser = _build_parser()
    args = parser.parse_args(argv)

    if args.command == "canvas":
        return _cmd_canvas(args.events, args.poll_ms)
    if args.command == "check-events":
        return _cmd_check_events(args.events)
    if args.command == "schema":
        return _cmd_schema()

    parser.error("unknown command")
    return 2


if __name__ == "__main__":
    raise SystemExit(main())
