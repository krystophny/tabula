# tabula

Minimal Python prototype for one behavior slice:

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

## Validate events only

```bash
tabula check-events --events .tabula/canvas-events.jsonl
```

## Print schema

```bash
tabula schema
```
