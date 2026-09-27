---
id: UC-12-UI
artifact: use-case-ui-design
status: current
last_reviewed: 2026-09-24
related: UC-12, UC-16, UC-05
---

# UC-12 UI — Application Variables & Secrets

## UX outcome

One Application Settings page has only two tabs: `Staging` and `Production`.
Each tab shows both `Environment variables` and `Secrets` as separate sections
on the same page. A key may exist in only one Environment. Editing values or
deleting or renaming a key creates a pending change, never an immediate runtime
change.

## Read in this order

1. [Screens and layout](screens.md)
2. [States and warnings](states.md)
3. [UC-12 specification](../specification.md)
4. [UC-16 workload picker](../../UC-16/ui/README.md)
5. [Shared shell](../../../architecture/ui/README.md)

API mapping and interactive product implementation follow contract design.
