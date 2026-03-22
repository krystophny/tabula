# UI Contract

Tabura now treats shared UI design as a first-class source of truth, not just an after-the-fact test artifact.

## Layered Source Of Truth

- Component contract:
  `internal/web/static/tabura-circle-contract.ts`
- Interaction flows:
  `tests/flows/`
- Target mapping per platform:
  `tests/flows/targets.cjs`

Each platform may render with its own native toolkit, but it must preserve the same semantic contract. The web runtime is not the source of truth because it is HTML; it is the source of truth only where it implements the shared contract.

## Tabura Circle Contract

The Tabura Circle contract currently defines:

- stable segment ids for `dialogue`, `meeting`, `silent`, `prompt`, `text_note`, `pointer`, `highlight`, and `ink`
- icon-only rendering with accessible labels and tooltips
- a corner enum of `top_left`, `top_right`, `bottom_left`, `bottom_right`
- corner-aware quarter-fan geometry computed from one anchor and one set of polar layout tuples
- local per-device persistence for circle placement
- bug reporting as a top-panel action instead of a second floating control

## Platform Rule

Web, iOS, and Android must share:

- ids and accessibility identifiers
- icon meaning
- active/inactive state semantics
- corner placement semantics
- interaction flows

Web, iOS, and Android do not need to share:

- DOM structure
- CSS layout primitives
- widget classes
- animation implementation details

Native clients should use toolkit-native implementations that realize the shared contract instead of recreating browser markup inside native shells.
