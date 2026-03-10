# Contributing

Tabura is pre-stable and still young. Optimize for the best product and codebase,
not for speculative compatibility.

## Rewrite Policy

- Do not assume real external API consumers or compatibility obligations unless
  there is concrete evidence.
- Breaking changes, API removals, schema changes, renames, deletions, and UX
  rewrites are allowed and encouraged when they materially improve UX, code
  quality, or maintainability.
- Prefer rewriting or deleting stale code, stale docs, stale tests, stale
  endpoints, and stale issue assumptions over preserving them.
- Compatibility layers, migration shims, and deprecation periods require
  explicit justification. They are not the default.
- If an API, architecture, workflow, or scope premise is weak, replace it
  rather than preserving it for speculative compatibility.
- Historical docs and release notes may remain as records, but they must not
  steer new design if they conflict with the current direction.

## Domain Replacement Policy

- The Workspace/Artifact/Item/Actor model replaces the older
  project/session/message model. Treat coexistence as a migration step, not a
  target architecture.
- When the new model covers an old table, handler, route, or UI flow, delete
  the old path instead of preserving both.
- Do not add bridges, adapters, compatibility shims, or dual-write paths just
  to keep legacy code alive. Those require explicit, concrete justification.
- The final schema and API surface should get smaller and clearer over time, not
  accumulate parallel legacy and replacement shapes.
- Reviewer default: ask whether the old thing can be deleted now. If the answer
  is yes, delete it instead of layering more compatibility around it.

## Primary Criteria

The only standing criteria for change are:

- excellence in UX
- code quality
- maintainability

## Practical Guidance

- Favor simpler public-core designs over compatibility baggage.
- Delete dead paths aggressively.
- Shrink or remove legacy integration surfaces unless they still earn their
  keep.
- Keep docs aligned with shipped reality, not with obsolete plans.
- If a cleanup is radical but clearly improves the product and codebase, do it.
