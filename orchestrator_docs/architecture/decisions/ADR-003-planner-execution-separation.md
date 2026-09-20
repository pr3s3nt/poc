# ADR-003 — Separate Planner, Resource Provisioning and Workload Deployment

Status: Accepted — 2026-09-20.

## Decision

- `PlanningService` is deterministic and side-effect free.
- UC-08 executes only resource nodes and returns validated outputs/target.
- UC-06 resolves workload bindings, renders manifests and applies workloads after UC-08 completes.
- A workload node may remain in the graph for dependency visualization, but it is excluded from resource-execution batches.

## Consequences

- Challenge Echo workload Definition is not copied into product execution.
- UC-05 and UC-06 reuse the same planner.
- Terraform contract inspection stays in planning; apply/state/output collection stays in UC-08.
