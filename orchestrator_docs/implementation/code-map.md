---
id: IMPLEMENTATION-CODE-MAP
artifact: design-to-code-map
status: current
last_reviewed: 2026-09-21
---

# Design-to-code map

| Design concern | Implementation entry point |
|---|---|
| Process bootstrap and adapter wiring | [`backend/internal/bootstrap`](../../backend/internal/bootstrap/) |
| HTTP API and `/ui/` delivery | [`backend/internal/delivery/http`](../../backend/internal/delivery/http/) |
| UC-01..UC-04 seeded executable baseline | [`backend/internal/seed`](../../backend/internal/seed/) and [`backend/internal/adapters/store`](../../backend/internal/adapters/store/) |
| UC-05 planning core used by the future preview flow | [`backend/internal/planning`](../../backend/internal/planning/) |
| UC-06, UC-07 and UC-09 orchestration/query | [`backend/internal/application/deployment`](../../backend/internal/application/deployment/) |
| UC-08 resource provisioning | [`backend/internal/application/provisioning`](../../backend/internal/application/provisioning/) |
| Application/Connection domain | [`backend/internal/domain/application`](../../backend/internal/domain/application/) |
| Environment/Deployment Set domain | [`backend/internal/domain/environment`](../../backend/internal/domain/environment/) |
| Deployment lifecycle/read model | [`backend/internal/domain/deployment`](../../backend/internal/domain/deployment/) |
| Resource contracts and lifecycle | [`backend/internal/domain/resource`](../../backend/internal/domain/resource/) |
| Score/delta/graph/matching/batches | [`backend/internal/planning`](../../backend/internal/planning/) |
| Persistence ports | [`backend/internal/ports/persistence`](../../backend/internal/ports/persistence/) |
| Execution ports | [`backend/internal/ports/execution`](../../backend/internal/ports/execution/) |
| In-memory + JSON snapshot store | [`backend/internal/adapters/store`](../../backend/internal/adapters/store/) |
| Fake walking-skeleton adapters | [`backend/internal/adapters/fake`](../../backend/internal/adapters/fake/) |
| Kubernetes executor/deployer | [`backend/internal/adapters/kubernetes`](../../backend/internal/adapters/kubernetes/) |
| Terraform executor/modules/inspection | [`backend/internal/adapters/terraform`](../../backend/internal/adapters/terraform/) |
| Seed catalog and profiles | [`backend/internal/seed`](../../backend/internal/seed/) |
| Web Console shell/features/shared UI | [`frontend/src`](../../frontend/src/) |
| Acceptance workloads | [`backend/examples/acceptance-app`](../../backend/examples/acceptance-app/) |
| HTTP end-to-end tests | [`backend/test/e2e`](../../backend/test/e2e/) |
| Planner challenge conformance | [`backend/test/conformance`](../../backend/test/conformance/) |
| kind/AWS verification | [`backend/test/integration`](../../backend/test/integration/) |

Nếu path/module thay đổi, cập nhật map này cùng imports, build tooling, runbook và
links trong cùng logical change.
