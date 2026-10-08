---
id: UC-12-REALIZATION
artifact: use-case-realization
status: draft
last_reviewed: 2026-09-27
---

# UC-12 — Use Case Realization (conceptual)

This sketch traces the approved [specification](specification.md). The shared
provider boundary and Vault implementation are described in
[ADR-006](../../architecture/decisions/ADR-006-application-configuration-provider.md).
Kubernetes Secret delivery follows
[ADR-008](../../architecture/decisions/ADR-008-vso-native-secret-delivery.md).
HTTP and snapshot operations are specified in the shared operation contract.

## Responsibilities

- Application settings boundary: select Environment, show both variable and
  secret sections, submit add, update, rename or delete, and show
  warnings/affected workloads.
- Configuration control: validate scoped key uniqueness and record a pending
  desired change; do not mutate runtime.
- Reference lookup: find workloads in the same Environment that refer to the
  selected key. It informs warnings; it does not rewrite workload references.
- Configuration control: maintain a desired revision per Environment and an
  applied revision per workload. New values belong to an immutable pending
  revision; an applied revision is not overwritten on save. Key names share
  one namespace across Variables and Secrets.
- Configuration provider: persist immutable key values under Application and
  Environment scope in Vault KV v2. The state store holds opaque value refs
  inside immutable revision metadata, not raw values. Variable reads may
  expose values; secret reads expose only name/configured status at the
  application boundary.
- Workload runtime: Vault bundles only the selected keys for a pinned deploy.
  VSO synchronizes that bundle to a namespace-local Kubernetes Secret. The
  Pod uses `secretKeyRef` for each environment variable; a new Pod is required
  to consume a changed value. The legacy Agent file path remains supported
  outside VSO mode.

## Trace main flow

| Step | Collaboration |
|---|---|
| MS-01 | Load both variable and secret keys for the selected Environment. |
| MS-02–MS-03 | Validate and record a new/changed value under the scoped key. |
| MS-04 | Read affected workload names and report pending Preview → Deploy. |
| VAR-01–VAR-02 | Look up consumers, warn, accept confirmation, keep workload bindings unchanged. |
| VAR-03 | Accept replacement secret value; never return it through a read path. |

## Boundary

UC-12 does not execute workload updates. Preview validates the desired
configuration and UC-16 workload references against a pinned revision and
draft version. A stale Preview cannot deploy. Deploy updates per-workload
applied revisions and restarts only affected workloads. Partial failure is
reported with individual outcomes and can be retried.

## Revised provider boundary (ADR-012)

Environment secret-store selection supersedes Application-wide provider. Variables
are revision values in Orchestrator; Secrets are immutable refs with owning store.
SecretStoreResolver resolves old refs as well as current selection. Store switch
claims Environment, copies desired values outside transaction, then atomically
commits new revision/selection. Old applied refs/bundles remain unchanged. VSO
connection/auth objects are store-specific; next Preview marks secret consumers
pending even when bytes are equal. See [ADR-012](../../architecture/decisions/ADR-012-environment-stores-and-transitions.md).
