---
id: UC-04-CONTEXT
artifact: use-case-context
status: current
last_reviewed: 2026-10-09
---

# UC-04 context — Configure execution connections

## Delivery state

Kubernetes onboarding by name + upload/paste kubeconfig + context selection is
implemented. Inspect/register API/UI, scoped Vault KV v2 credential store,
PostgreSQL migration/backfill/race/reopen and direct executor resolution are
locally verified. The [2026-10-06 evidence](../../verification/2026-10-06-uc04-kubeconfig-upload.md)
includes live read-only kind verification to READY and a reviewed published
video. The registration video uses fake workload adapters and does not prove
live workload deployment on the uploaded Connection.

The [desktop UI correction](../../verification/2026-10-06-uc04-connections-ui.md)
records the revised form controls/list layout and reviewed replacement recording.

Existing host-context connections/legacy API remain compatible. Default
Connection selection does not change Organization defaults. Internal cluster
binding follows ADR-013 without per-cluster Definition registration; other
UC-03 matching Definitions can reference new Connections. AWS identity for infrastructure provisioning remains future
scope under [ADR-009](../../architecture/decisions/ADR-009-aws-access-key-storage.md).
Fleet fixed-cluster delivery rejects credential-backed targets.

## Read in this order

1. [Specification](specification.md)
2. [Realization](realization.md)
3. [Sequence](sequence.puml)
4. [VOPC](vopc.puml)
5. [Connection state machine](../../architecture/state-machines/connection.puml)

UI: [screens](ui/screens.md), [states](ui/states.md), [HTTP mapping](ui/api-mapping.md).

## Implementation entry points

- [`shared credential design`](../../architecture/connection-credentials.md)
- [`backend/internal/application/connection`](../../../backend/internal/application/connection/)
- [`backend/internal/ports/credentials`](../../../backend/internal/ports/credentials/)
- [`backend/internal/adapters/vault/connection_credentials.go`](../../../backend/internal/adapters/vault/connection_credentials.go)
- [`backend/internal/domain/application`](../../../backend/internal/domain/application/)
- [`backend/internal/adapters/kubernetes`](../../../backend/internal/adapters/kubernetes/)
- [`backend/internal/adapters/terraform`](../../../backend/internal/adapters/terraform/)
- [`backend/internal/bootstrap`](../../../backend/internal/bootstrap/)
- [`backend/internal/seed`](../../../backend/internal/seed/)

## Current change in progress

[ADR-012](../../architecture/decisions/ADR-012-environment-stores-and-transitions.md)
supersedes permanent target lock/Application provider selection. Specifications
now include editable per-Environment destinations and Vault stores/transitions;
implementation status is tracked in CURRENT_STATE, not inferred from old evidence.

## Implicit internal cluster delivery (2026-10-09)

[ADR-013](../../architecture/decisions/ADR-013-implicit-existing-cluster.md)
removes per-cluster user Definition registration/matching for internal-k8s.
Default planning, system binding, persistence admission and Console projection
are implemented and [locally verified](../../verification/2026-10-09-implicit-existing-cluster-local.md).
Other resource/cloud matching remains unchanged.
[Real kind verification](../../verification/2026-10-09-k8s4f-live.md) now covers
retained `k8s-4f` registration and isolated real-adapter FE/BE/PostgreSQL execution
with the builtin binding. No cloud proof is claimed.
