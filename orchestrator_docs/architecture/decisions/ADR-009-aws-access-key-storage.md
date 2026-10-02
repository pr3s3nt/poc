---
id: ADR-009
artifact: architecture-decision
status: current
last_reviewed: 2026-10-02
---

# ADR-009 — AWS access-key credential storage

Status: Accepted
Date: 2026-10-02

## Context

UC-04 AWS registration is designed but its credential mechanism and storage
were open. The user selected AWS access keys and approved Vault storage.
This decision does not claim AWS registration or its full verification is
already implemented.

## Decision

1. The first AWS credential flow uses an access key ID and secret access key.
   IAM role onboarding is not required for this first flow. Credentials enter
   only a dedicated UC-04 write request, never Resource Definition variables.
2. Store the pair in Vault KV v2 under a dedicated Organization/Connection
   credential namespace, separate from UC-12 Application values and workload
   bundles. Connection records keep only an opaque reference. UC-12 workload
   roles must have no access to this namespace.
3. Credential writes use unique immutable object IDs; duplicate/concurrent
   registration must never overwrite another Connection's key. A failed
   database insert must remove only that attempt's credential object. Failed
   cleanup is an operational error, with a safe reference and no key bytes.
4. Read APIs, errors, logs, snapshots, Preview, manifests and Git must not expose
   either submitted key. Secret reads are internal, scope-checked and fail
   closed. No fallback to host AWS credentials for a registered key reference.
5. Verification and execution resolve credentials only for the selected
   Organization/Connection. Subprocesses receive them through a scoped process
   environment, never command arguments, Terraform variable files or global
   process environment. No workload may receive these account credentials.
6. AWS registration still requires UC-04 identity, region/backend and minimum
   permission verification before READY. STS identity success alone is not
   proof of provisioning permissions. Durable Terraform state, rotation and
   production RBAC retain their existing release/deferred boundaries.

## Delivery boundary

The current local hardening change implements catalog/Driver Inputs and UC-16
validation first. AWS credential selection is now resolved. Full AWS onboarding
requires the separate verifier, credential lifecycle, executor wiring and UI
contract to be completed before advertising an AWS registration endpoint.
No live AWS/Vault/cluster operation is authorized by this ADR.

## References

- [UC-04 specification](../../usecase/UC-04/specification.md)
- [UC-04 realization](../../usecase/UC-04/realization.md)
- [Operation contracts](../contracts/operation-contracts.md)
- [Durable Terraform state backlog](../../backlog/D01-durable-terraform-state.md)
