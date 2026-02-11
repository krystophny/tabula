# tabula

Minimal Python prototype focused on a terminal Codex session + separate canvas window.

- start in `prompt` mode
- switch to `discussion` when a valid canvas artifact event arrives
- return to `prompt` on `clear_canvas`

Codex stays in your terminal session. The canvas window is separate.

## Event bridge

Append JSONL events to `.tabula/canvas-events.jsonl`.

Supported kinds:

- `text_artifact`
- `image_artifact`
- `pdf_artifact`
- `clear_canvas`

## Run

```bash
python -m pip install -e .[test]
python -m pip install -e .[gui]   # optional, for canvas window
tabula canvas --events .tabula/canvas-events.jsonl
```

## Bootstrap protocol for a project

Creates/updates:
- `AGENTS.md` protocol block for Codex
- `.tabula/prompt-injection.txt` for extra prompt injection
- `.tabula/artifacts/draft.md` as markdown artifact target
- `.tabula/canvas-events.jsonl` bridge file
- `.gitignore` binary-artifact ignore patterns
- runs `git init` if `.git/` does not exist

```bash
tabula bootstrap --project-dir /path/to/project
```

## Interactive markdown MVP flow (Codex + Pandoc + git)

This runs in your terminal with interactive `codex` sessions and no REPL takeover.

Workflow:
1. Codex draft round writes markdown.
2. Pandoc renders PDF.
3. Codex revision round updates markdown once (unless `--skip-revision`).
4. Pandoc renders PDF again.
5. JSONL `pdf_artifact` events are appended for canvas display.
6. Git commits markdown only (`.tabula/artifacts/draft.md`).

```bash
tabula markdown-mvp \
  --project-dir /path/to/project \
  --mode project \
  --prompt "Create a short markdown note about X and revise once"
```

Use `--mode global` to run Codex with `--skip-git-repo-check`.

## Validate events only

```bash
tabula check-events --events .tabula/canvas-events.jsonl
```

## Print schema

```bash
tabula schema
```

## Real-tool integration tests (optional)

```bash
TABULA_REAL_TOOLS=1 PYTHONPATH=src python -m pytest tests/integration/test_real_optional_tools.py::test_real_pandoc_render_markdown_to_pdf
TABULA_REAL_CODEX=1 PYTHONPATH=src python -m pytest tests/integration/test_real_optional_tools.py::test_real_codex_exec_writes_output_file
```
