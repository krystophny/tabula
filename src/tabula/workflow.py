from __future__ import annotations

import json
import shutil
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Literal
from uuid import uuid4

from .protocol import (
    DEFAULT_MARKDOWN_PATH,
    DEFAULT_PDF_PATH,
    EVENTS_PATH,
    BootstrapResult,
    bootstrap_project,
    commit_markdown_only,
    load_injection_text,
)
from .runner import RunResult, SubprocessRunner

CodexMode = Literal["project", "global"]


@dataclass(frozen=True)
class WorkflowResult:
    returncode: int
    message: str


def build_codex_prompt(
    user_prompt: str,
    *,
    markdown_path: Path,
    pdf_path: Path,
    injection_text: str,
    round_name: str,
) -> str:
    segments = [
        f"Round: {round_name}",
        "You are running inside Tabula markdown flow.",
        f"Primary artifact file: {markdown_path.as_posix()}",
        f"PDF target file: {pdf_path.as_posix()}",
        "Constraints:",
        "- Update markdown artifact only.",
        "- Keep output concise and practical.",
        "- Do not commit files yourself.",
        f"Task: {user_prompt}",
    ]
    if round_name == "revision":
        segments.insert(1, "Revise existing markdown once based on current contents.")
    if injection_text:
        segments.extend(["Extra instructions:", injection_text])
    return "\n".join(segments)


def build_codex_command(prompt: str, *, project_dir: Path, mode: CodexMode) -> list[str]:
    cmd = ["codex", "-C", str(project_dir)]
    if mode == "global":
        cmd.append("--skip-git-repo-check")
    cmd.append(prompt)
    return cmd


def run_codex_interactive(prompt: str, *, project_dir: Path, mode: CodexMode, runner: SubprocessRunner) -> RunResult:
    cmd = build_codex_command(prompt, project_dir=project_dir, mode=mode)
    return runner.run(cmd, cwd=project_dir, capture=False)


def render_markdown_to_pdf(markdown_path: Path, pdf_path: Path, runner: SubprocessRunner) -> RunResult:
    if shutil.which("pandoc") is None:
        return RunResult(returncode=2, stderr="pandoc not found in PATH\n")

    pdf_path.parent.mkdir(parents=True, exist_ok=True)
    cmd = ["pandoc", str(markdown_path), "-o", str(pdf_path)]
    return runner.run(cmd, cwd=markdown_path.parent, capture=True)


def append_pdf_event(events_path: Path, *, title: str, pdf_path: Path, page: int = 0) -> None:
    payload = {
        "event_id": str(uuid4()),
        "ts": datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
        "kind": "pdf_artifact",
        "title": title,
        "path": str(pdf_path),
        "page": page,
    }
    events_path.parent.mkdir(parents=True, exist_ok=True)
    with events_path.open("a", encoding="utf-8") as handle:
        handle.write(json.dumps(payload) + "\n")


def run_markdown_mvp(
    *,
    user_prompt: str,
    project_dir: Path,
    mode: CodexMode,
    commit_message: str,
    skip_revision: bool = False,
    markdown_rel: Path = DEFAULT_MARKDOWN_PATH,
    pdf_rel: Path = DEFAULT_PDF_PATH,
    events_rel: Path = EVENTS_PATH,
    runner: SubprocessRunner | None = None,
) -> WorkflowResult:
    command_runner = runner or SubprocessRunner()
    bootstrap: BootstrapResult = bootstrap_project(
        project_dir,
        markdown_rel=markdown_rel,
        pdf_rel=pdf_rel,
        events_rel=events_rel,
        runner=command_runner,
    )
    paths = bootstrap.paths
    injection_text = load_injection_text(paths.injection_path)

    rounds = ["draft"] if skip_revision else ["draft", "revision"]
    for idx, round_name in enumerate(rounds, start=1):
        prompt = build_codex_prompt(
            user_prompt,
            markdown_path=paths.markdown_path,
            pdf_path=paths.pdf_path,
            injection_text=injection_text,
            round_name=round_name,
        )
        codex_result = run_codex_interactive(prompt, project_dir=paths.project_dir, mode=mode, runner=command_runner)
        if codex_result.returncode != 0:
            return WorkflowResult(returncode=codex_result.returncode, message=f"codex {round_name} failed")

        pandoc_result = render_markdown_to_pdf(paths.markdown_path, paths.pdf_path, command_runner)
        if pandoc_result.returncode != 0:
            return WorkflowResult(returncode=pandoc_result.returncode, message=pandoc_result.stderr.strip() or "pandoc failed")
        append_pdf_event(paths.events_path, title=f"markdown {round_name} ({idx}/{len(rounds)})", pdf_path=paths.pdf_path)

    commit = commit_markdown_only(paths.project_dir, paths.markdown_path, commit_message, command_runner)
    if commit.returncode != 0:
        return WorkflowResult(returncode=commit.returncode, message=commit.stderr.strip() or "git commit failed")
    return WorkflowResult(returncode=0, message="markdown flow completed")
