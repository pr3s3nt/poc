---
id: ADR-INDEX
artifact: decision-index
status: current
last_reviewed: 2026-10-24
---

# Architecture Decision Records

- [ADR-022 — Account-scoped readiness diagnostics](ADR-022-account-diagnostics.md) — user-authorized implementation; real scoped probes, durable observations and explicit owned execution checks.

- [ADR-001 — Historical profile enrichment; retained descriptor/scopes](ADR-001-profile-resource-scopes.md) — enrichment/guard superseded by ADR-016/017.
- [ADR-002 — External calls outside database transactions](ADR-002-transaction-boundaries.md)
- [ADR-003 — Separate planner, resource provisioning and workload deployment](ADR-003-planner-execution-separation.md)
- [ADR-004 — React web console served from the Go backend origin](ADR-004-react-web-console.md)
- [ADR-005 — Root backend and frontend source boundaries](ADR-005-root-backend-frontend-layout.md)
- [ADR-006 — Per-Application configuration provider and Vault Agent delivery](ADR-006-application-configuration-provider.md)
- [ADR-007 — Fleet GitRepo workload delivery on internal kind](ADR-007-fleet-gitrepo-workload-delivery.md)
- [ADR-008 — VSO native Secret delivery for UC-12](ADR-008-vso-native-secret-delivery.md)

- [ADR-009 — AWS access-key credential storage](ADR-009-aws-access-key-storage.md)

## Workload rendering

- [ADR-010 — Definition-selected workload rendering with score-k8s](ADR-010-score-k8s-workload-rendering.md)
  — current required v2 rendering boundary; placement follows ADR-016/017 for Internal/EKS targets.

- [ADR-011 — Environment execution binding set once](ADR-011-environment-execution-binding.md) — superseded; current targets follow ADR-016/017, state/scope identities follow ADR-014/schema.

- [ADR-012 — Environment stores and transitions](ADR-012-environment-stores-and-transitions.md) — current Secret Store/operation/generation guards; deployment selector/target transition partially superseded by ADR-016/017 and retired in V06.08.

- [ADR-013 — Implicit existing-cluster node from Environment binding](ADR-013-implicit-existing-cluster.md) — superseded by ADR-016; historical builtin binding only, not current matching behavior.

- [ADR-014 — Terraform v2 lifecycle/modules/state](ADR-014-terraform-v2-lifecycle.md) — lifecycle/modules/state implemented and verified in M04; Definition-driven replacement evidence in V04–V06 handoff.

- [ADR-015 — Infrastructure accounts for Definition-selected drivers](ADR-015-infrastructure-driver-accounts.md) — V01 account catalog; downstream Definition consumers delivered through V06.


- [ADR-016 — Definition-selected workload lifecycle and targets](ADR-016-definition-selected-workload-targets.md) — accepted V03 option B; internal cluster matching replaces ADR-013; per-resource targets with one-cluster V03 delivery boundary.

- [ADR-017 — Definition dependencies and deferred cluster targets](ADR-017-definition-dependency-execution.md) — V04–V06 contract; same-execution EKS and gated profile removal.

- [ADR-018 — Explicit EC2 instance-role Driver Accounts](ADR-018-ec2-role-driver-accounts.md) — IAM User-free Terraform execution, issue #6.

- [ADR-019 — Visual deployment review and persisted progress](ADR-019-visual-deployment-review.md).

- [ADR-020 — Runtime message catalogs for the Web Console](ADR-020-console-message-catalogs.md) — Vietnamese default + English; new languages are catalog files, not code.

- [ADR-021 — Git-owned Kubernetes manifest rendering policy](ADR-021-git-render-bundles.md) — accepted design; normalization/defaults/provisioners/patches in Git, infrastructure unchanged; implementation pending.

- [ADR-024 — Open workload object set, inventory and Gateway API routes](ADR-024-open-workload-objects.md) — supersedes the ADR-021 object envelope for Git bundles.

- [ADR-023 — Native basic observability](ADR-023-native-observability.md): platform collection, Metrics Server and durable application history.

- [ADR-025 — Workload catalog, CronJob và persistent storage](ADR-025-workload-catalog.md).
