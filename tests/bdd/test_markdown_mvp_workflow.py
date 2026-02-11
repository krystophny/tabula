from __future__ import annotations

import json
from dataclasses import dataclass
from pathlib import Path

import tabula.workflow as workflow
from tabula.runner import RunResult
from tabula.workflow import build_codex_command, run_markdown_mvp


@dataclass
class FakeRunner:
    status_output: str = "M .tabula/artifacts/draft.md\n"
    fail_pandoc: bool = False

    def __post_init__(self) -> None:
        self.calls: list[list[str]] = []
        self.config: dict[str, str] = {}

    def run(self, argv: list[str], *, cwd: Path | None = None, capture: bool = False) -> RunResult:
        self.calls.append(argv)

        if argv and argv[0] == "codex":
            return RunResult(returncode=0)
        if argv and argv[0] == "pandoc":
            if self.fail_pandoc:
                return RunResult(returncode=1, stderr="pandoc failed\n")
            return RunResult(returncode=0)

        if argv[:2] == ["git", "init"]:
            return RunResult(returncode=0, stdout="initialized\n")
        if argv[:2] == ["git", "add"]:
            return RunResult(returncode=0)
        if argv[:3] == ["git", "status", "--porcelain"]:
            return RunResult(returncode=0, stdout=self.status_output)
        if argv[:3] == ["git", "commit", "-m"]:
            return RunResult(returncode=0, stdout="commit ok\n")

        if argv[:3] == ["git", "config", "user.name"]:
            if len(argv) == 3:
                if "user.name" in self.config:
                    return RunResult(returncode=0, stdout=self.config["user.name"] + "\n")
                return RunResult(returncode=1, stderr="unset\n")
            self.config["user.name"] = argv[3]
            return RunResult(returncode=0)

        if argv[:3] == ["git", "config", "user.email"]:
            if len(argv) == 3:
                if "user.email" in self.config:
                    return RunResult(returncode=0, stdout=self.config["user.email"] + "\n")
                return RunResult(returncode=1, stderr="unset\n")
            self.config["user.email"] = argv[3]
            return RunResult(returncode=0)

        return RunResult(returncode=0)


def test_given_project_mode_when_running_markdown_mvp_then_two_codex_rounds_render_pdf_and_markdown_commit(
    tmp_path: Path, monkeypatch
) -> None:
    runner = FakeRunner()
    monkeypatch.setattr(workflow.shutil, "which", lambda _: "/usr/bin/pandoc")

    result = run_markdown_mvp(
        user_prompt="Write a short markdown summary",
        project_dir=tmp_path,
        mode="project",
        commit_message="docs: update markdown artifact",
        runner=runner,
    )

    assert result.returncode == 0
    assert result.message == "markdown flow completed"

    codex_calls = [call for call in runner.calls if call and call[0] == "codex"]
    assert len(codex_calls) == 2
    assert all("--skip-git-repo-check" not in call for call in codex_calls)

    pandoc_calls = [call for call in runner.calls if call and call[0] == "pandoc"]
    assert len(pandoc_calls) == 2

    events_path = tmp_path / ".tabula" / "canvas-events.jsonl"
    lines = [line for line in events_path.read_text(encoding="utf-8").splitlines() if line.strip()]
    assert len(lines) == 2
    payloads = [json.loads(line) for line in lines]
    assert all(item["kind"] == "pdf_artifact" for item in payloads)

    add_calls = [call for call in runner.calls if call[:2] == ["git", "add"]]
    commit_calls = [call for call in runner.calls if call[:2] == ["git", "commit"]]
    assert add_calls[-1][-1] == ".tabula/artifacts/draft.md"
    assert commit_calls[-1][-1] == ".tabula/artifacts/draft.md"


def test_given_global_mode_when_running_markdown_mvp_then_codex_is_invoked_with_skip_repo_check(
    tmp_path: Path, monkeypatch
) -> None:
    runner = FakeRunner()
    monkeypatch.setattr(workflow.shutil, "which", lambda _: "/usr/bin/pandoc")

    result = run_markdown_mvp(
        user_prompt="Write a short markdown summary",
        project_dir=tmp_path,
        mode="global",
        commit_message="docs: update markdown artifact",
        skip_revision=True,
        runner=runner,
    )

    assert result.returncode == 0
    codex_calls = [call for call in runner.calls if call and call[0] == "codex"]
    assert len(codex_calls) == 1
    assert "--skip-git-repo-check" in codex_calls[0]


def test_given_missing_pandoc_when_running_markdown_mvp_then_workflow_fails_before_commit(
    tmp_path: Path, monkeypatch
) -> None:
    runner = FakeRunner()
    monkeypatch.setattr(workflow.shutil, "which", lambda _: None)

    result = run_markdown_mvp(
        user_prompt="Write markdown",
        project_dir=tmp_path,
        mode="project",
        commit_message="docs: update markdown artifact",
        skip_revision=True,
        runner=runner,
    )

    assert result.returncode == 2
    assert "pandoc not found" in result.message
    assert not [call for call in runner.calls if call[:2] == ["git", "commit"]]


def test_build_codex_command_supports_project_and_global_modes(tmp_path: Path) -> None:
    project_cmd = build_codex_command("hello", project_dir=tmp_path, mode="project")
    global_cmd = build_codex_command("hello", project_dir=tmp_path, mode="global")

    assert project_cmd[:3] == ["codex", "-C", str(tmp_path)]
    assert "--skip-git-repo-check" not in project_cmd

    assert global_cmd[:3] == ["codex", "-C", str(tmp_path)]
    assert "--skip-git-repo-check" in global_cmd
