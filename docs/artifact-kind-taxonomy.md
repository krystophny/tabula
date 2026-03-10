# Artifact Kind Taxonomy

This is the canonical artifact-kind contract for the shipped runtime.

Rules:

- Every artifact kind renders through the canonical canvas surfaces only: `text_artifact`, `pdf_artifact`, or `image_artifact`.
- Artifact kinds may change copy, metadata, and available actions, but they may not invent a separate gesture model or a parallel mini-app.
- Items stay the open-loop tracker. Artifacts stay the thing on canvas. Special kinds only narrow which canonical actions are emphasized.

## Canonical actions

- `open_show`
- `annotate_capture`
- `compose`
- `bundle_review`
- `dispatch_execute`
- `track_item`
- `delegate_actor`

## Current kinds

| Kind | Family | Canvas surface | Canonical emphasis |
| --- | --- | --- | --- |
| `document`, `markdown`, `pdf`, `image`, `reference` | reference | text / pdf / image | open, annotate, review, track |
| `transcript` | transcript | text | open, annotate, review, track |
| `plan_note`, `idea_note`, `external_note` | planning note / captured note | text | open, annotate, compose, review, track |
| `annotation` | review bundle | text | open, annotate, review, dispatch, track |
| `github_issue`, `github_pr` | proposal / review artifact | text | open, annotate, review, dispatch, track, delegate |
| `external_task` | action card | text | open, compose, dispatch, track, delegate |
| `email`, `email_thread` | message | text | open, annotate, compose, dispatch, track |

## Boundary

- Mail artifacts may expose mail actions, but they still live on the same text canvas and do not get a separate interaction grammar.
- Review artifacts may expose review or dispatch controls, but they still route through the same canvas, prompt, and item semantics.
- Planning notes and transcripts remain text artifacts; they do not get bespoke panes or alternate gesture meanings.

Authority in code:

- `internal/web/static/artifact-taxonomy.js`
- `internal/web/static/app-item-sidebar-artifacts.js`
- `internal/web/static/canvas-actions.js`
- `tests/playwright/artifact-kind-taxonomy.spec.ts`
