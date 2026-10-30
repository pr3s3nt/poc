---
id: ADR-030
artifact: architecture-decision
status: current
last_reviewed: 2026-10-28
related: [ADR-007, ADR-014, ADR-016, ADR-017, ADR-024]
---

# ADR-030 — Optional CD providers and infrastructure reconciliation

## Decision and authorization

User decisions on 2026-10-28: design a provider-neutral delivery boundary with
Fleet first and Argo CD/Flux addable through adapters; CD use is **optional**,
enabled explicitly for each Environment, independently for staging/production.
This record specifies the implementation target. It does not claim delivery,
authorize cluster changes, or activate an unrelated roadmap iteration.

Fleet version is pinned to **v0.15.2**, per the subsequent user decision.
Terraform retains all infrastructure/dependency ownership, including Kubernetes
StatefulSets for PostgreSQL/Redis created by its modules. Only workload manifests
and Environment public routes are published to Fleet GitRepos. Task-owned E2E
setup installs Fleet v0.15.2 in its isolated cluster; product deployment requires
an explicitly installed/verified controller and target registration.

## Context

Current `execution.WorkloadDeployer` offers Apply/WaitReady/Remove. The old
`adapters/gitops` implementation assumes a host context, Harbor, a single
Deployment and an in-memory revision map. Its credential guard rejects all
targets. Bootstrap also selects delivery process-wide and type-asserts Fleet
for routes. These are insufficient for durable, optional Environment delivery.
Terraform owns infrastructure; native observability collects runtime health,
not desired/live configuration equivalence.

## Decisions

1. Environment has desired and applied delivery selections. New and existing
   Environments default to `DIRECT`. Checking **Dùng CD provider** exposes a
   verified provider binding; Fleet is the first implementation. Save records
   intent only; Preview → Deploy applies it. Provider choice does not select or
   change a workload's Definition-derived cluster/namespace or Terraform driver.
2. `DeliveryProvider` models provider identity/capabilities, revision submission,
   observation, reconciliation request, withdrawal and ownership handoff.
   Concrete Fleet/Argo/Flux objects never enter the planner or public lifecycle
   enums. Direct delivery implements the same lifecycle with explicit limited
   capabilities. Git publication is a reusable port, not part of the Fleet API.
3. Store immutable approved artifacts, operation intents, provider handles,
   inventories and observations. Distinguish an admitted execution revision
   from the last successful revision; background observation cannot silently
   roll a failed published candidate back to the successful one or adopt a draft.
4. Sync and self-heal are separate policies. CD selection permits syncing
   admitted deployments. Optional **Tự sửa sai lệch** defaults off and is
   explicitly enabled by PE/Admin for supported continuous objects. Infrastructure
   is detect/review only; there is no unattended Terraform apply or destruction.
5. One writer owns each object/field. VSO/Agent, Terraform, HPA and Kubernetes
   child controllers keep their existing ownership. Fleet owns continuous
   workload desired state; Orchestrator coordinates revisions and retains
   guarded cleanup authority. Never run a competing direct apply loop.
6. Fleet bindings name an explicit management account, workspace and an exact
   registered target identity. Manager and destination can differ. The first
   release requires Fleet and target registration to be preinstalled; no
   automatic cluster enrollment, Fleet installation or arbitrary target selector.
   Same-execution new EKS cannot use Fleet until it is explicitly registered and
   verified; reject that combination before provisioning, retain direct support.
7. Provider change on an occupied Environment is an explicit ownership
   transition, including direct ↔ Fleet. It preserves cluster/namespace/data,
   excludes simultaneous workload/config/resource changes, and fences all
   writers. Unknown outcomes hold the operation for explicit recovery. Fleet ↔
   future providers requires their verified handoff capabilities, never fallback.
8. Preserve ADR-024 ordering and UID cleanup. Fleet packages are split by
   lifecycle phase; Jobs are not continuously healed or implicitly rerun.
   Fleet/Helm native deletion cannot replace the existing UID-precondition
   guarantee. Use retained objects, controller withdrawal and shared guarded
   deletion as specified in the contract; failure to prove these semantics on
   the pinned Fleet/Helm release blocks the adapter's release.
9. Terraform scans use a persisted encrypted execution-input snapshot and the
   original state/module/provider pins. Scan results are separate from execution
   status and never rewrite state/outputs/current Set. Reviewed remediation uses
   saved plans with fresh state guards, dependency validation and existing
   destructive-update restrictions. D12/D13/D14 remain deferred.

## Platform CD configuration — user decision 2026-10-29

PE/Admin author bindings in a dedicated platform page, including verification,
immutable versions and Environment usage. Fleet supports explicit managed GitRepos
or a preexisting GitRepo with a dedicated generated subtree. Existing controllers,
registrations, GitRepo and reader Secrets are retained. Provider topology includes
separate controller and agent namespaces. UI and adapter schemas remain provider
specific behind the generic binding/lifecycle boundary. Environment opt-in and
Terraform ownership decisions above remain unchanged.

## Alternatives and consequences

Extending the old global `fleet-gitrepo` flag would still couple Environment
behavior and cleanup to one CD implementation. A generic `Apply(any)` interface
would hide revision, ownership and recovery requirements. Replacing Terraform
with Crossplane would change the infrastructure authority and is outside scope.

This design adds durable delivery/outbox/scan records and scoped provider setup.
It is more than a rename of the existing adapter. Provider extensions still need
credentials, configuration schema, capability verification and conformance tests;
“adapter only” does not mean identical semantics or zero integration work.

## Supersession and routes

For the new delivery path this replaces ADR-007's fixed-kind/global configuration,
in-memory revision handling and Fleet-native workload deletion. Its no-secret
Git boundary remains. ADR-024's primary-workload rendering constraints remain;
CronJob support here means an extra object, not a new standalone Score workload.
Current Fleet unavailability remains true until implementation/acceptance.

- [Delivery contract](../contracts/delivery-reconciliation.md).
- [Infrastructure drift contract](../contracts/infrastructure-drift.md).
- [Implementation plan and release gates](../../implementation/delivery-reconciliation.md).
- [Schema](../database/schema.md#delivery-and-drift--adr-027-required-design).
- [Current state](../../CURRENT_STATE.md) and
  [IMP-039](../../implementation/deviations.md#imp-039--delivery-reconciliation-design-2026-10-28).
