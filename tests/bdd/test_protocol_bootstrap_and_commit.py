from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path

from tabula.protocol import (
    AGENTS_PROTOCOL_BEGIN,
    AGENTS_PROTOCOL_END,
    bootstrap_project,
    commit_markdown_only,
)
from tabula.runner import RunResult


@dataclass
class FakeRunner:
    status_output: str = "M .tabula/artifacts/draft.md\n"
    fail_commit: bool = False

    def __post_init__(self) -> None:
        self.calls: list[list[str]] = []
        self.config: dict[str, str] = {}

    def run(self, argv: list[str], *, cwd: Path | None = None, capture: bool = False) -> RunResult:
        self.calls.append(argv)

        if argv[:2] == ["git", "init"]:
            return RunResult(returncode=0, stdout="initialized\n")

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

        if argv[:3] == ["git", "status", "--porcelain"]:
            return RunResult(returncode=0, stdout=self.status_output)

        if argv[:3] == ["git", "commit", "-m"]:
            if self.fail_commit:
                return RunResult(returncode=1, stderr="commit failed\n")
            return RunResult(returncode=0, stdout="commit ok\n")

        if argv[:2] == ["git", "add"]:
            return RunResult(returncode=0)

        return RunResult(returncode=0)


def test_given_new_project_when_bootstrapped_then_git_agents_and_binary_ignores_are_created(tmp_path: Path) -> None:
    runner = FakeRunner()
    result = bootstrap_project(tmp_path, runner=runner)

    assert result.git_initialized is True
    assert (tmp_path / ".gitignore").exists()
    assert (tmp_path / ".tabula" / "artifacts" / "draft.md").exists()
    assert (tmp_path / ".tabula" / "prompt-injection.txt").exists()
    assert (tmp_path / "AGENTS.md").exists()

    agents = (tmp_path / "AGENTS.md").read_text(encoding="utf-8")
    assert AGENTS_PROTOCOL_BEGIN in agents
    assert AGENTS_PROTOCOL_END in agents
    assert ".tabula/artifacts/draft.md" in agents

    gitignore = (tmp_path / ".gitignore").read_text(encoding="utf-8")
    assert ".tabula/artifacts/*.pdf" in gitignore
    assert ".tabula/artifacts/*.png" in gitignore


def test_given_existing_agents_when_bootstrapped_then_protocol_block_is_upserted_without_losing_custom_text(
    tmp_path: Path,
) -> None:
    custom = "# AGENTS\n\nCustom section\n\n"
    (tmp_path / "AGENTS.md").write_text(custom, encoding="utf-8")
    runner = FakeRunner()

    bootstrap_project(tmp_path, runner=runner)
    content = (tmp_path / "AGENTS.md").read_text(encoding="utf-8")

    assert "Custom section" in content
    assert content.count(AGENTS_PROTOCOL_BEGIN) == 1
    assert content.count(AGENTS_PROTOCOL_END) == 1


def test_given_markdown_changes_when_committing_then_only_markdown_path_is_staged_and_committed(tmp_path: Path) -> None:
    runner = FakeRunner(status_output="M .tabula/artifacts/draft.md\n")
    bootstrap_project(tmp_path, runner=runner)
    md = tmp_path / ".tabula" / "artifacts" / "draft.md"
    md.write_text("# Hello\n", encoding="utf-8")

    result = commit_markdown_only(tmp_path, md, "docs: update markdown artifact", runner)

    assert result.returncode == 0
    add_calls = [call for call in runner.calls if call[:2] == ["git", "add"]]
    commit_calls = [call for call in runner.calls if call[:2] == ["git", "commit"]]
    assert add_calls
    assert commit_calls
    assert add_calls[-1][-1] == ".tabula/artifacts/draft.md"
    assert commit_calls[-1][-1] == ".tabula/artifacts/draft.md"


def test_given_no_markdown_changes_when_committing_then_no_commit_is_created(tmp_path: Path) -> None:
    runner = FakeRunner(status_output="")
    bootstrap_project(tmp_path, runner=runner)
    md = tmp_path / ".tabula" / "artifacts" / "draft.md"

    result = commit_markdown_only(tmp_path, md, "docs: update markdown artifact", runner)
    assert result.returncode == 0
    assert "no markdown changes to commit" in result.stdout
    assert not [call for call in runner.calls if call[:2] == ["git", "commit"]]
