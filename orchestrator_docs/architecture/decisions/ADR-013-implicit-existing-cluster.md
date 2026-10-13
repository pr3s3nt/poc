---
id: ADR-013
artifact: architecture-decision
status: current
last_reviewed: 2026-10-13
---

# ADR-013 — Implicit existing-cluster node from Environment binding

Status: Accepted — 2026-10-09. Supersedes the internal cluster Definition
registration/matching requirement in ADR-001/ADR-011; ADR-012 editable bindings
and target generation remain authoritative.

## Decision

For `internal-k8s`, profile enrichment creates the existing `k8s-cluster.internal`
provider node directly from the configured Environment Connection. No user
registration or criteria matching is required for this cluster node. Keep its
current connection-addressed descriptor `k8s-cluster.internal#connections.<key>`
and current Application scope so existing Active Resource identities are preserved. Environment
binding/generation/version is pinned by the existing plan/Preview contract;
a transition selects its own destination binding and cannot reuse source-target
state. Do not change the existing cluster logical key merely to introduce system binding.

The node stays visible in graph, Preview, provider-first batches, progress and
history. It uses the `existing-cluster` executor through the same UC-08 pipeline.
Execution validates the scoped READY Kubernetes Connection and resolves private
credentials at invocation time. It checks/resolves an existing cluster, never
creates or destroys it. Downstream Kubernetes resources consume the same target
contract as an EKS provider. Missing/unconfigured/wrong-kind/cross-organization
Connections fail closed; no host/default fallback for credential-backed targets.

## Internal Definition and persistence compatibility

Use a reserved system-owned Definition identity `builtin-existing-cluster`
with profile `internal-k8s`, type `k8s-cluster`, driver `existing-cluster`, no
Connection override, no resource references/provision rules. Its driver variables
are connection context values (`name`, `kubeContext`). The effective Definition
is constructed by trusted code, not selected from matching criteria. The cluster
Match carries the selected Environment Connection key explicitly. User-supplied
params, references or provision rules cannot override this implicit node's
connection/context; reject conflicting cluster params before execution. Hash/Preview
must deterministically distinguish this system binding from a catalog match.

Persist the internal Definition idempotently at deployment/provisioning admission
before resource progress/Active Resource writes, outside pure planning. Existing
Definition foreign keys remain valid; no schema change is required. Never
materialize it during Preview. Reject every public registration with driver `existing-cluster`, regardless of
key, connection, profile or criteria, before insert. The reserved key remains
protected for every driver;
validate an existing record against the exact expected system definition and
fail on collision rather than overwrite user content. Historical cluster
Definitions/resources/deployment snapshots remain readable and are not deleted
by this change; retained dev catalog records are removed only by the separately
authorized T29 reset, with no new deletion API or registration compatibility;
new execution ignores authored existing-cluster Definitions for this implicit
node. The system record must not become an editable matching policy.

T22 supersedes host-context compatibility: only explicit scoped KUBECONFIG
Connections execute internal Kubernetes. Keep existing descriptors. Reopen/retry/remove
and destination transitions keep stored target identity and scoped credentials.
Conformance fixtures without product profile enrichment keep their reference
matching behavior; do not rewrite historical verification evidence.

## Cloud and application resource boundary

For `aws-eks`, VPC/EKS continue to use profile-filtered matching Definitions and
Terraform, including Environment account guards and ENVIRONMENT infrastructure scope only
(T27). The internal cluster Application resource scope is distinct from the
removed LEGACY_APPLICATION execution-binding mode. Per UC-03 BR-08, public VPC/EKS Definitions omit ConnectionKey and execution inherits the pinned READY Environment AWS account; conflicting malformed catalog entries remain guarded. Theo ADR-014, Aurora/ElastiCache cũng dùng cùng Environment AWS
account/region; không explicit account override.
VPC supplies EKS and Aurora; workloads wait for Kubernetes target and required
resource outputs. Do not force Aurora to depend on EKS or run in Kubernetes.

All other resources retain Definition matching and driver contracts: internal
PostgreSQL is StatefulSet/Service/PVC, cloud PostgreSQL is Aurora. Both expose
Resource Type outputs to consumers. Namespace/database/cache provisioning dùng Terraform modules theo ADR-014;
score-k8s rendering bắt buộc và AWS Environment binding theo các contract hiện hành.

## Validation

Prove no cluster Definition is needed, authored matching/ties cannot redirect
this node, deterministic read-only Preview, binding pinning, READY/kind/org
checks, real persistence FK admission behavior without external mutation,
scoped target restoration, old host-target rejection, retry/remove/transition correctness and unchanged
AWS VPC/EKS/Aurora matching/dependency contracts. Live cluster/cloud verification
requires separate user scope; local checks do not prove live connectivity.
