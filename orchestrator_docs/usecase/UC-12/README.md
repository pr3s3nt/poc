---
id: UC-12-CONTEXT
artifact: use-case-context
status: current
last_reviewed: 2026-09-27
---

# UC-12 context — Manage Application variables and secrets

## Delivery state

Specification and UI design approved. Application Settings, authenticated
UC-12 API, immutable desired revisions and Vault KV v2 adapter are implemented;
secret values are redacted from read responses and excluded from state snapshots.
The Vault-backed provider decision is recorded in
[ADR-006](../../architecture/decisions/ADR-006-application-configuration-provider.md).
Runtime reference resolution, scoped Vault workload access, VSO-to-Kubernetes
Secret delivery and Preview → Deploy are implemented and verified on kind. The persistent kind
Vault uses KV v2 and Kubernetes auth; the backend uses a scoped token kept
outside the repository. Vault is not HA and requires manual unseal after restart.
The legacy Agent path remains available outside VSO mode; see
[ADR-008](../../architecture/decisions/ADR-008-vso-native-secret-delivery.md).

## Read in this order

1. [Specification](specification.md)
2. [UI design](ui/README.md)
3. [Realization](realization.md), [sequence](sequence.puml) and [VOPC](vopc.puml)
4. [UC-16 workload configuration](../UC-16/README.md)
5. [UC-05 preview](../UC-05/README.md)

See [VSO kind verification](../../verification/2026-09-28-uc12-vso-kind.md) for
the observed VSO path and its limitations.
