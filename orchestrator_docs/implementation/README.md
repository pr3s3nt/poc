---
id: IMPLEMENTATION-INDEX
artifact: implementation-index
status: current
last_reviewed: 2026-10-24
---

# Implementation documentation

## Current implementation

Go product code nằm trong [`backend/`](../../backend/); Web Console nằm trong
[`frontend/`](../../frontend/). Build/run guidance bắt đầu tại
[backend README](../../backend/README.md) và
[frontend README](../../frontend/README.md); thao tác có external side effect
phải theo [operations index](../operations/README.md).

## Navigation

- [Account diagnostics implementation and real acceptance](account-diagnostics.md) — authorized 2026-10-25; delivery tracked separately from design.

- [DEVEX-ENV implementation handoff — implemented 2026-10-24](workload-environment-editor.md)

- [Design-to-code map](code-map.md)
- [Known design/implementation deviations](deviations.md)
- [Humanitec and Score compatibility matrix](humanitec-compatibility.md)
- [Backend and frontend layout](package-layout.md)
- [UC-06 planner reference analysis](uc06-planner-reference.md)
- [Documentation reconciliation](documentation-reconciliation.md)

Source code và tests sở hữu actual implemented behavior nhưng không tự thay đổi
required product behavior trong specification.

- [Terraform v2 rule reconciliation](v2-rule-reconciliation.md) — ID/canonical destinations và a897d6b/3367f7e history, không code completion.

- [Git render bundles implementation handoff](git-render-bundles.md) — ADR-021 work breakdown and real acceptance gate; not implementation evidence.
