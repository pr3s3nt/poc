---
id: ADR-010
artifact: architecture-decision
status: current
last_reviewed: 2026-10-06
---

# ADR-010 — Definition-selected workload rendering with score-k8s

Status: Accepted — 2026-10-06. Implementation authorized; live verification is separate.

## Context

The user selected the direction of provisioning infrastructure first, then using
its outputs to generate workload manifests. The user authorized full product integration on 2026-10-06.
This decision does not authorize external verification.

[ADR-003](ADR-003-planner-execution-separation.md) separates deterministic
planning, UC-08 resource provisioning and UC-06 workload rendering/delivery.
The current Go renderer hard-codes Deployment/Service/Secret generation.
The current catalog rejects unsupported driver/type pairs, and workload nodes
are not ordinary provision batches. The baseline used only the native renderer before this extension.

## Decision

1. Add a Definition-selected workload rendering boundary to UC-06. A Definition
   of type `workload` selects a renderer, initially the existing renderer or a
   score-k8s adapter. Registration, matching and plan pinning remain owned by
   Orchestrator. Rendering does not become a UC-08 infrastructure operation.
2. UC-08 continues to own cluster resolution, namespace, PostgreSQL, Terraform
   state and Active Resources. score-k8s must not create or reconcile those
   resources, select a database implementation, generate credentials or destroy
   resources. Its provisioners for dependencies only expose already-resolved
   outputs and Secret references.
3. Keep the current `modules.*` workload identity, class `default` and weighted
   five-field matching. This is a product extension inspired by Generic
   Workloads, not a claim of Humanitec Generic Workload/API compatibility.
   Do not introduce Humanitec `workloads.*` identities or a new compute class
   during the initial implementation.
4. score-k8s is a replaceable implementation behind a rendering port. Direct
   Kubernetes/Fleet delivery, readiness, per-workload results, route reconciliation
   and final current-set commit stay in the existing application services.
5. Driver Inputs select a platform rendering bundle by stable ID. Each plan pins
   the installed immutable bundle snapshot. The snapshot
   fixes the exact renderer version, ordered output-only provisioners and patch
   templates. Definition registration does not accept arbitrary commands,
   mutable remote URLs or credentials. Registration fields and validation are
   in the [rendering contract](../contracts/workload-rendering.md).
6. Normalize the current product Score subset before rendering. Preserve probe,
   replicas, Service-reference, CPU/memory, UC-12 secret and public-route behavior;
   using score-k8s does not automatically widen the accepted public Score schema.
7. Preview pins rendering intent and non-secret inputs. Unprovisioned outputs
   remain unresolved; Preview must not provision or claim to have produced exact
   final manifests. Deploy uses the same pinned Definition/bundle/configuration,
   resolves outputs and validates the generated manifests before any workload
   apply. A bundle/Definition change makes pending Preview stale.
8. Use an isolated disposable score-k8s workspace per render. All resource
   identity and durable state remain with Orchestrator/Kubernetes/Vault. Do not
   persist or publish `.score-k8s` state. Stable workload names and output-only
   provisioners are required so fresh workspace GUIDs cannot change deployment
   identity. Stateful/random provisioners are excluded from this first slice.
9. `render_bundle` is a stable selector ID, not an immutable snapshot. Each Plan
   pins the immutable bundle installed in the running process (version, binary
   digest, adapter/provisioner digest). The Definition's server-owned
   `sourceFingerprint` records the bundle digest seen at registration as audit
   provenance only; it is not a runtime equality gate and remains part of the
   Definition content hash. After an upgrade A → B the same Definition selects
   B; Preview/Deploy tokens pinned to A are stale and pending detects the
   renderer-only change. No Definition update/retire operation is added.
   Unavailable IDs, ambiguous matches and binary drift still fail.
10. Keep the current renderer available for existing deployments and comparison.
   Enable score-k8s only by explicit reviewed selection. A failed score-k8s render
   fails the deployment; it must not silently fall back to a different renderer.

## Ownership

| Responsibility | Owner |
|---|---|
| Dependency graph, Definition selection, private/shared identity | Planner |
| Database, namespace and validated resource outputs | UC-08 executors |
| Secret materialization and scoped secret references | Existing secret/Vault/Kubernetes adapters |
| Score normalization, output mapping, manifest validation | Workload rendering adapter |
| Manifest conversion and reviewed patches | score-k8s |
| Apply, readiness, Fleet revision and Environment routes | UC-06 delivery adapters |
| Current set and execution history | Orchestrator persistence |

## Consequences and alternatives

- Reuse a maintained Score renderer instead of implementing another template
  engine. Custom Template Driver and arbitrary compute targets are not included.
- Adapter work is still necessary: score-k8s does not understand product
  descriptors, UC-12 keys, pending tokens or the current Score extensions.
- Replacing all provisioning with score-k8s would duplicate identity/state and
  change cloud behavior; that alternative is outside this implementation.
- Definition-selected rendering requires changes to catalog validation, workload
  matching, immutable planning intent and persistence review. Existing JSON
  columns alone are not evidence that no migration will be needed.

## Acceptance and implementation boundary

Registration, planner pinning, rendering and pending-update behavior follow the
[rendering contract](../contracts/workload-rendering.md). Local tests run the
configured CLI with fake infrastructure/delivery and verify output semantics,
Secret references, immutable snapshots and stale-preview rejection.
Live kind/AWS verification requires a separately authorized task.

## Sources

- [score-k8s CLI](https://docs.score.dev/docs/score-implementation/score-k8s/cli/)
  documents initialization, manifest generation and potentially sensitive local state.
- [Provisioners](https://docs.score.dev/docs/score-implementation/score-k8s/resources-provisioners/)
  document output-only integration mechanisms and encoded Kubernetes Secret references.
- [Patch templates](https://docs.score.dev/docs/score-implementation/score-k8s/patch-templates/)
  document post-generation patches; their syntax is distinct from the product Delta JSON Patch.
- [Humanitec Generic Workload inputs](https://developer.humanitec.com/platform-orchestrator/docs/platform-orchestrator/working-with/workloads/)
  explain the conceptual separation of workload specification and driver execution.

## Initial implementation choices

Use the existing installed score-k8s 0.15.0 CLI through an explicit backend
`-score-k8s` path. The binary is inspected at startup; its content digest and
version become part of the immutable rendering bundle and plan hash. No runtime
download or global process environment is used. Embedded platform patches and
output-only binding templates form `score-k8s-internal-v1`. The current native
renderer is an explicit built-in selection when no workload Definition matches;
matching score-k8s Definitions override it, with ambiguity still rejected.

This slice supports internal-k8s only. Existing output binding resolves database
outputs and Service references before render. The adapter translates resolved
variables into output-only bindings and encoded Secret references, rather than
letting score-k8s match/provision original infrastructure dependencies. Both
VSO and legacy Agent delivery retain their current semantics.
