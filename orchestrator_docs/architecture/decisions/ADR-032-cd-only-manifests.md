---
id: ADR-032
artifact: architecture-decision
status: current
last_reviewed: 2026-11-02
related: [ADR-030, ADR-008, ADR-024, ADR-027]
---

# ADR-032 — CD owns application manifests

## Decision

The user approved retiring Direct application delivery on 2026-11-02.
Real deployments require a verified CD binding per Environment. Orchestrator
plans, renders, admits and publishes revisions; the CD provider owns application
manifest submission, drift correction and lifecycle. Terraform retains its
infrastructure/dependency resources, including its database StatefulSets/PVCs.
Secret controllers retain responsibility for materializing secret values.

This supersedes ADR-030's optional CD/default Direct behavior. Saving Settings
does not deploy. Self-heal remains an independent opt-in. Binding changes still
require reviewed ownership transfer; provider failure never authorizes direct
apply. Jobs retain completion receipts and phase ordering.

## Complete manifest boundary

The CD artifact includes nonsecret renderer auxiliaries and workload-scoped VSO
ServiceAccount, VaultConnection, VaultAuth and VaultStaticSecret declarations.
Dependency secret values are written to an immutable Vault bundle along with
Application secret references, or immutable private value references for Vault
Agent delivery. They never enter rendered Git payloads. A public
CA-only Secret may accompany VSO configuration, validated as certificate-only;
arbitrary Secret objects remain forbidden. VSO destination Secrets are children
of VaultStaticSecret and are not CD desired inventory.

VSO connection/auth objects and CD secret-delivery ServiceAccounts are
revision/workload scoped to avoid collisions with historical direct auxiliaries and release
ownership collisions. The prerequisite phase must observe VSO readiness before
Jobs or rollout. All new application manifest cleanup uses the CD provider's
retained-withdrawal and UID-checked removal/prune contract, including auxiliaries.

## Existing installations and simulation

Do not rewrite released migrations, infer CD bindings, reset databases or delete
existing workloads. Stored DIRECT and empty legacy selections remain readable;
they mean CD must be configured before another real deployment. Historical
Direct artifacts retain narrowly scoped observation/export/removal support for
reviewed migration to CD and cleanup. No new Direct target is selectable.
An installation without a durable admitted baseline remains blocked for migration;
do not rerender or invent incarnation proof to take over existing objects.

New CD-owned inventories record that auxiliaries belong to CD. Older inventories
keep the existing guarded auxiliary/secret cleanup path until retired, so an
upgrade cannot silently abandon resources. There is no database schema change.
Explicit fake-adapter simulation remains available for local/unit tests and
cannot serve as evidence of a real CD deployment.

## Operations

Configure durable delivery Git storage, install/verify the provider and select a
READY binding before Preview/Deploy. The Console always shows the provider
selector; it has no CD enable/disable toggle. A missing binding is an actionable
configuration error. This decision grants no cluster/database mutation authority.
