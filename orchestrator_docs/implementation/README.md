---
id: IMPLEMENTATION-INDEX
artifact: implementation-index
status: current
last_reviewed: 2026-09-21
---

# Implementation documentation

## Current implementation

Go product code nằm trong [`backend/`](../../backend/); Web Console nằm trong
[`frontend/`](../../frontend/). Build/run guidance bắt đầu tại
[backend README](../../backend/README.md) và
[frontend README](../../frontend/README.md); thao tác có external side effect
phải theo [operations index](../operations/README.md).

## Navigation

- [Design-to-code map](code-map.md)
- [Known design/implementation deviations](deviations.md)
- [Backend and frontend layout](package-layout.md)
- [UC-06 planner reference analysis](uc06-planner-reference.md)
- [Documentation reconciliation](documentation-reconciliation.md)

Source code và tests sở hữu actual implemented behavior nhưng không tự thay đổi
required product behavior trong specification.
