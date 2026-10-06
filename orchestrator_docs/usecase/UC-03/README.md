---
id: UC-03-CONTEXT
artifact: use-case-context
status: current
last_reviewed: 2026-10-02
---

# UC-03 context — Register Resource Definition

## Delivery state

Runtime-supported Definition registration API/UI and normalized PostgreSQL
persistence are implemented and locally verified, including atomic criteria,
duplicate guards and no-restart matching. Driver Inputs policy in specification BR-11–BR-14 is implemented and locally
verified on 2026-10-02; the score-k8s extension is described below.

## Definition-selected workload rendering

Definitions of type `workload` can select the installed score-k8s rendering
bundle for `internal-k8s`. Registration validates the pair, strict inputs and
server-owned fingerprint; it does not accept inline templates or commands.

See [ADR-010](../../architecture/decisions/ADR-010-score-k8s-workload-rendering.md)
and the [rendering contract](../../architecture/contracts/workload-rendering.md).
Local verification does not claim live kind/AWS execution.

## Read in this order

1. [Specification](specification.md)
2. [Realization](realization.md)
3. [Sequence](sequence.puml)
4. [VOPC](vopc.puml)
5. [Planner reference](../../implementation/uc06-planner-reference.md)
6. [Humanitec/Score compatibility matrix](../../implementation/humanitec-compatibility.md)

UI: [screens](ui/screens.md), [states](ui/states.md), [HTTP mapping](ui/api-mapping.md).

## Implementation entry points

- [`backend/internal/domain/resource`](../../../backend/internal/domain/resource/)
- [`backend/internal/planning`](../../../backend/internal/planning/)
- [`backend/internal/seed`](../../../backend/internal/seed/)
