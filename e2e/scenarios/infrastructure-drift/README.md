# Infrastructure drift through Console

Implements [UC-08 INF](../../../orchestrator_docs/usecase/UC-08/specification.md#infrastructure-drift--adr-027)
and [INF-C01–04](../../../orchestrator_docs/architecture/contracts/infrastructure-drift.md).
`flow.mjs` is invoked by the ordinary-form application runner with
`ORCH_RECON_FOCUS=infrastructure`; it reuses that runner's platform setup,
application/workload/dependency forms and business assertions. It does not repeat
provider handoff scenarios already accepted by the delivery run.

Prerequisites and common connection/credential setup:
[delivery runner](../delivery-reconciliation/README.md), Node/Playwright dependencies
from [E2E guide](../../README.md), task-owned Kubernetes/Fleet/Git/Vault platform,
and a **fresh** run-tagged Compose database/image with `INFRASTRUCTURE_DRIFT=true`.
Never upgrade/reset a retained test database implicitly. The private proof file
`drift-compose-project` must name `orch-reconciliation-drift-20261029`, and the
cluster/container/namespace UID must match the existing private run evidence.
No shared cluster, cloud or company application is authorized by this runner.

Run from repository root with the delivery runner's variables, a new
`ORCH_RECON_STATE_FILE`, distinct `ORCH_RECON_APP_NAME`, and
`ORCH_RECON_FOCUS=infrastructure`; leave `ORCH_RECON_OBSERVATIONS` unset. Product
mutations use only ordinary Console buttons/dialogs; no Score/JSON/YAML input or
product mutation API substitutes for UI. Technical reads verify acknowledgements.

Assertions: a complete initial scan covers all managed resources, reports the
unchanged namespace IN_SYNC and no proposed actions/output changes. Provider
computed/defaulted PVC/StatefulSet fields can appear as external drift without
proposed actions; they remain visible per INF-C02, never filtered to force green. The
scan leaves canonical encrypted-state bytes/serial/lineage and current Set
unchanged. One reversible task-owned namespace provenance annotation edit is the
explicitly authorized core drift assertion. A new UI scan detects an in-place
update without adopting state. Review displays safe actions/paths, confirmation
requires its checkbox, and repair restores the annotation with unchanged UID and
lineage, stable outputs/current Set and a newly acknowledged state serial. A fresh
scan shows the repaired namespace IN_SYNC with no proposed actions for any
instance, and application checks plus old/new queued jobs still pass.

Side effects: the application/dependencies from the shared ordinary-form runner,
one controlled namespace annotation drift, stored scan/review/operation evidence.
On failure retain the owned target/private logs and diagnose; do not manually
repair the attribute, reset state, weaken assertions or rerun uncertain effects.
Acknowledged phase flags resume completed work without repeating the edit.

Cleanup: run the delivery scenario's `cleanup.mjs` with the same private state and
connection. It removes workloads and application through UI, verifies namespace/PV
absence and retains parent/sibling resources. Platform teardown is separate and
uses exact cluster/Compose incarnation proofs. Logs, screenshots, credentials,
kubeconfigs and mutable state remain outside Git. A passing parser/unit test or
setup alone is not real infrastructure acceptance.
