---
id: ADR-INDEX
artifact: decision-index
status: current
last_reviewed: 2026-10-09
---

# Architecture Decision Records

- [ADR-001 — Execution-profile resource enrichment and scopes](ADR-001-profile-resource-scopes.md)
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
  — accepted; internal-k8s registration/Preview/Deploy integration, with separate live verification.

- [ADR-011 — Environment execution binding set once](ADR-011-environment-execution-binding.md) — independent Kubernetes/AWS Environment targets, new Environment-scoped VPC/EKS and explicit legacy compatibility.

- [ADR-012 — Environment stores and transitions](ADR-012-environment-stores-and-transitions.md) — editable destinations, atomic admission, Vault transfer and PostgreSQL migration with downtime.

- [ADR-013 — Implicit existing-cluster node from Environment binding](ADR-013-implicit-existing-cluster.md) — internal Kubernetes cluster node bypasses user Definition matching; cloud/resource matching remains.
