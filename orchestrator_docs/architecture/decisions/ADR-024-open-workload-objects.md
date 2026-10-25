---
id: ADR-024
artifact: architecture-decision
status: current
last_reviewed: 2026-10-25
related: [ADR-021, ADR-010, ADR-016]
---

# ADR-024 — Open workload object set, object inventory and Gateway API routes

## Decision and scope

User decision 2026-10-25: a Git render bundle must be able to deploy a
production application, not only one Deployment and one Service. Replace the
ADR-021 object/field envelope for **Git bundles** by an open namespaced object
set with a platform-owned object inventory. Add Kubernetes Gateway API
`HTTPRoute` as a platform public-route kind so Environments get a browser URL
on clusters that expose apps through a Gateway. Installed (legacy) bundles keep
their ADR-010 contract; infrastructure, Terraform, matching and secret delivery
are unchanged.

## Decisions

1. **Open object set.** A bundle may emit any namespaced Kubernetes object the
   target cluster serves: workloads (Deployment, StatefulSet, DaemonSet, Job,
   CronJob), HPA, PDB, ConfigMap, PVC, ServiceAccount, Role/RoleBinding,
   NetworkPolicy, HTTPRoute and namespaced custom resources of installed
   operators. Pod and container fields are no longer allowlisted: volumes,
   initContainers, sidecars, `envFrom`, lifecycle hooks and host/privileged
   settings are allowed; the cluster's Pod Security Admission and RBAC stay the
   enforcement point. Bundles are authored and selected by PE/Admin only.
2. **Kept boundaries.** No cluster-scoped object (checked against API discovery
   before any write), no `v1 Secret` and no VSO (`secrets.hashicorp.com`)
   object from a bundle: Secret bytes keep flowing only through Vault/VSO/Agent
   (ADR-006/008), because bundle repositories are public. Platform ownership
   labels are injected into every object and cannot be overridden.
3. **Primary workload invariants.** The object named after the workload is an
   `apps/v1` Deployment or StatefulSet carrying the platform identity labels and
   selector. Explicit Score intent (image, command/args, probes, explicit
   requests/limits, replicas, approved environment bindings) stays protected;
   extra containers, environment entries and missing-value defaults (including
   limits) are bundle policy. The Service named after the workload exists
   exactly when the Score declares ports, uses the platform selector and
   includes every Score port.
4. **Extra-object stage.** `bundle.yaml` gains optional `resources`: ordered
   templates that each return an array of Kubernetes objects, evaluated after
   score-k8s generation and before patches (patches may modify them).
5. **Per-workload intent.** Score `metadata.annotations` (string map) are kept
   in the workload module and exposed as `.Input.workload.annotations`, the
   Score-native extension point bundles read for per-app settings (autoscaling
   bounds, migration commands, schedules, config files). They are not copied
   into manifests unless a bundle does so.
6. **Ownership and inventory.** Before applying, every non-auxiliary object
   must be absent or already owned by the same Application, Environment and
   workload; otherwise nothing is written. The workload instance records the
   applied objects (`apiVersion/kind/name` and owning deployment IDs). Objects
   that disappear from the rendered set are deleted after a successful rollout;
   removal deletes every inventory object by UID precondition after verifying
   ownership, then performs the existing fixed removal. A failed deploy keeps
   the union of old and new objects in the inventory so nothing leaks.
7. **Readiness and order.** If the set contains Jobs, all non-rollout objects
   are applied first and every Job must complete (a failed Job fails the
   deploy) before Deployments/StatefulSets/DaemonSets are applied; rollout
   status is then awaited for every rollout object. Without Jobs the previous
   single apply is kept. Other kinds are ready when applied.
8. **Gateway API routes.** Runtime configuration selects the public-route kind:
   `ingress` (default, unchanged) or `httproute` with a Gateway parent
   `namespace/name[/sectionName]`; the base domain becomes configurable. The
   platform owns one `HTTPRoute orch-public` per Environment namespace with the
   Environment host and one `PathPrefix` rule per Score public route. DNS, TLS
   certificates and Gateway listeners remain cluster setup. Bundles may emit
   additional HTTPRoutes for advanced routing.

## Rationale

The ADR-021 envelope was a lifecycle limitation, not a product requirement:
removal knew only three fixed objects, so any additional object would leak.
Recording an inventory, checking ownership before writes and deleting by UID
removes that cause while keeping the safety properties that shared namespaces
need. Secret and cluster-scope boundaries protect other tenants and the public
Git model without restricting what an application can run.

## Consequences and supersession

This ADR supersedes, for Git bundles only, ADR-021 "Keep the current
object/lifecycle envelope", RB-C04 step 5 "no creation of additional objects"
and the object/field rules of RB-C05, and UC-06/UC-07 RB "no new kinds". The
detailed contract is the [Git bundle contract](../contracts/git-render-bundles.md#rb-c08--open-object-set-adr-024).
Fleet GitRepo delivery stays unavailable (it already fails closed).

## Canonical routes

- Behavior: UC-06 `RB-05..07`, UC-07 `RB-03`.
- [Contract RB-C08/RB-C09](../contracts/git-render-bundles.md#rb-c08--open-object-set-adr-024).
- [Public routes operations](../../operations/public-routes.md).
