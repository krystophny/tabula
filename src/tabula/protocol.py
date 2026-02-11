from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path

from .runner import RunResult, SubprocessRunner

TABULA_DIR = Path(".tabula")
ARTIFACT_DIR = TABULA_DIR / "artifacts"
EVENTS_PATH = TABULA_DIR / "canvas-events.jsonl"
INJECTION_PATH = TABULA_DIR / "prompt-injection.txt"
DEFAULT_MARKDOWN_PATH = ARTIFACT_DIR / "draft.md"
DEFAULT_PDF_PATH = ARTIFACT_DIR / "draft.pdf"

AGENTS_PROTOCOL_BEGIN = "<!-- TABULA_PROTOCOL:BEGIN -->"
AGENTS_PROTOCOL_END = "<!-- TABULA_PROTOCOL:END -->"

GITIGNORE_BINARY_PATTERNS = [
    ".tabula/artifacts/*.pdf",
    ".tabula/artifacts/*.png",
    ".tabula/artifacts/*.jpg",
    ".tabula/artifacts/*.jpeg",
    ".tabula/artifacts/*.gif",
]


@dataclass(frozen=True)
class ProjectPaths:
    project_dir: Path
    markdown_path: Path
    pdf_path: Path
    events_path: Path
    injection_path: Path
    agents_path: Path


@dataclass(frozen=True)
class BootstrapResult:
    paths: ProjectPaths
    git_initialized: bool


def _protocol_block(markdown_rel: Path, pdf_rel: Path, events_rel: Path, injection_rel: Path) -> str:
    lines = [
        AGENTS_PROTOCOL_BEGIN,
        "## Tabula Codex Protocol",
        "",
        "Use this protocol for markdown->pdf artifact flow in this project.",
        "",
        f"1. Read extra instructions from `{injection_rel.as_posix()}` and apply them.",
        f"2. Write markdown artifact only to `{markdown_rel.as_posix()}` unless user explicitly overrides.",
        f"3. Render/update PDF at `{pdf_rel.as_posix()}` when asked.",
        f"4. Emit canvas artifact events as JSONL lines in `{events_rel.as_posix()}`.",
        "5. Never stage or commit binary artifacts from `.tabula/artifacts/*`.",
        "6. Commits in this flow must include markdown files only.",
        "",
        AGENTS_PROTOCOL_END,
        "",
    ]
    return "\n".join(lines)


def _upsert_protocol_block(existing: str, block: str) -> str:
    if AGENTS_PROTOCOL_BEGIN in existing and AGENTS_PROTOCOL_END in existing:
        prefix, remainder = existing.split(AGENTS_PROTOCOL_BEGIN, 1)
        _, suffix = remainder.split(AGENTS_PROTOCOL_END, 1)
        merged = prefix.rstrip() + "\n\n" + block + suffix.lstrip("\n")
        return merged
    if not existing.strip():
        return block
    return existing.rstrip() + "\n\n" + block


def _ensure_gitignore(project_dir: Path) -> None:
    path = project_dir / ".gitignore"
    existing_lines: list[str]
    if path.exists():
        existing_lines = path.read_text(encoding="utf-8").splitlines()
    else:
        existing_lines = []

    seen = set(existing_lines)
    appended = [pattern for pattern in GITIGNORE_BINARY_PATTERNS if pattern not in seen]
    if not appended:
        return

    if existing_lines and existing_lines[-1].strip():
        existing_lines.append("")
    existing_lines.extend(appended)
    path.write_text("\n".join(existing_lines) + "\n", encoding="utf-8")


def _ensure_git_repo(project_dir: Path, runner: SubprocessRunner) -> bool:
    if (project_dir / ".git").exists():
        return False
    result = runner.run(["git", "init"], cwd=project_dir, capture=True)
    if result.returncode != 0:
        message = result.stderr.strip() or result.stdout.strip() or "git init failed"
        raise RuntimeError(message)
    return True


def bootstrap_project(
    project_dir: Path,
    *,
    markdown_rel: Path = DEFAULT_MARKDOWN_PATH,
    pdf_rel: Path = DEFAULT_PDF_PATH,
    events_rel: Path = EVENTS_PATH,
    injection_rel: Path = INJECTION_PATH,
    runner: SubprocessRunner | None = None,
) -> BootstrapResult:
    command_runner = runner or SubprocessRunner()
    project_dir = project_dir.resolve()
    project_dir.mkdir(parents=True, exist_ok=True)

    markdown_path = (project_dir / markdown_rel).resolve()
    pdf_path = (project_dir / pdf_rel).resolve()
    events_path = (project_dir / events_rel).resolve()
    injection_path = (project_dir / injection_rel).resolve()
    agents_path = (project_dir / "AGENTS.md").resolve()

    markdown_path.parent.mkdir(parents=True, exist_ok=True)
    events_path.parent.mkdir(parents=True, exist_ok=True)
    injection_path.parent.mkdir(parents=True, exist_ok=True)

    if not markdown_path.exists():
        markdown_path.write_text("# Draft\n\n", encoding="utf-8")
    if not events_path.exists():
        events_path.touch()
    if not injection_path.exists():
        injection_path.write_text(
            "Keep output concise. Follow project AGENTS.md protocol. Do not commit binary artifacts.\n",
            encoding="utf-8",
        )

    _ensure_gitignore(project_dir)

    block = _protocol_block(markdown_rel, pdf_rel, events_rel, injection_rel)
    if agents_path.exists():
        existing = agents_path.read_text(encoding="utf-8")
    else:
        existing = "# AGENTS\n\n"
    agents_path.write_text(_upsert_protocol_block(existing, block), encoding="utf-8")

    git_initialized = _ensure_git_repo(project_dir, command_runner)
    return BootstrapResult(
        paths=ProjectPaths(
            project_dir=project_dir,
            markdown_path=markdown_path,
            pdf_path=pdf_path,
            events_path=events_path,
            injection_path=injection_path,
            agents_path=agents_path,
        ),
        git_initialized=git_initialized,
    )


def load_injection_text(injection_path: Path) -> str:
    if not injection_path.exists():
        return ""
    return injection_path.read_text(encoding="utf-8").strip()


def ensure_git_identity(project_dir: Path, runner: SubprocessRunner) -> None:
    name = runner.run(["git", "config", "user.name"], cwd=project_dir, capture=True)
    email = runner.run(["git", "config", "user.email"], cwd=project_dir, capture=True)
    if name.returncode != 0 or not name.stdout.strip():
        runner.run(["git", "config", "user.name", "Tabula Bot"], cwd=project_dir, capture=True)
    if email.returncode != 0 or not email.stdout.strip():
        runner.run(["git", "config", "user.email", "tabula@example.local"], cwd=project_dir, capture=True)


def commit_markdown_only(project_dir: Path, markdown_path: Path, message: str, runner: SubprocessRunner) -> RunResult:
    rel_md = markdown_path.relative_to(project_dir).as_posix()
    add = runner.run(["git", "add", "--", rel_md], cwd=project_dir, capture=True)
    if add.returncode != 0:
        return add

    status = runner.run(["git", "status", "--porcelain", "--", rel_md], cwd=project_dir, capture=True)
    if status.returncode != 0:
        return status
    if not status.stdout.strip():
        return RunResult(returncode=0, stdout="no markdown changes to commit\n", stderr="")

    ensure_git_identity(project_dir, runner)
    commit = runner.run(["git", "commit", "-m", message, "--", rel_md], cwd=project_dir, capture=True)
    return commit
