---
id: ARCH-KUBERNETES-INSTALLATION
artifact: architecture-design
status: current
last_reviewed: 2026-11-18
---

# Independent Kubernetes installations — issue #91

Approved 2026-11-18: staging/production are independent installations, each with
PostgreSQL, Vault, backend/frontend, first-run setup and an HTTPRoute to an existing
Traefik Gateway. Agent verification is restricted to a new owned namespace on kind;
user runs staging/production. No shared Gateway/controller/DNS/TLS changes, main
installation mutation, data reset, HA claim or external-cloud deployment.

## Package and identity

One Kustomize base plus kind/staging/production overlays, operator tooling and
runbook. Each overlay chooses namespace/installation label, images, Gateway
namespace/name/listener, hostname, storage and resource sizing. A private local
configuration may supply reviewed overrides without editing checked-in samples.
Every mutating command requires explicit kubeconfig/context matching the intended
context; existing namespaces require matching ownership labels and the saved UID.
No adoption of preexisting unrelated data or cluster resources.

Three independent RWO PVCs hold PostgreSQL, Vault file storage and backend work/
cache/recovery data. No shared credentials PVC or mandatory node co-location.
Credentials are separate Kubernetes Secrets with least-consumer mounts. EmptyDir
initialization copies private files to the consuming UID with mode 0400/0600;
Secret projection permissions must not be weakened to satisfy owner-only readers.
Secrets, recovery material, kubeconfig, evidence and rendered Secret YAML stay
outside Git in operator-owned mode-0700 directories and mode-0600 files.

## Installation phases

1. Validate/render and prove target ownership/Gateway attachment prerequisites.
2. Prepare private credential/certificate inputs and insert missing Secrets only;
   rerun compares existing values, never rotates or replaces secrets silently.
3. Apply namespace/config/storage and PostgreSQL/Vault services. Wait for PostgreSQL
   readiness and Vault process reachability (sealed Vault is not ready for runtime).
4. Operator initializes/unseals Vault, retaining the init response and Shamir shares
   outside the cluster. User chose manual unseal on 2026-11-18. No unseal key/root
   token is persisted in a Pod, Secret, PVC, log or backend mount. Private root
   authorization is used only transiently through a task-owned port-forward.
   Configure KV/policies and issue three scoped periodic runtime/application tokens.
   With `vaultKubernetesAuth: true`, the installer also configures the
   `kubernetes` auth mount for this cluster. Only the Vault container mounts the
   rotating reviewer ServiceAccount token and Kubernetes CA. A named,
   installation-owned ClusterRole and binding grant only TokenReview creation.
   Renewal uses scoped tokens, never a retained root token. Vault restart requires
   explicit operator unseal with the matching private shares.
5. Execute the versioned owner database Job, wait for success and only then apply
   backend/frontend/HTTPRoute. Backend receives Console/State runtime credentials,
   independent state key, setup token, scoped Connection/output Vault tokens and
   public trust. It never mounts owner/superuser/root/unseal material.
6. Browser creates Organization/Admin using SETUP-01/02, then ordinary product
   configuration/Git init APIs. No fixed Admin, Organization or business catalog seed.

For a new staging installation, omitting the optional `tls` configuration selects
plaintext PostgreSQL and HTTP Vault on internal Services. Use this mode only on a
trusted cluster network because credentials and data traverse it unencrypted.
Keep the transport selected during preparation for the lifetime of the installation.
With `tls` configured, staging verifies PostgreSQL and Vault certificates against
operator-supplied trust. Production requires that TLS configuration. Kind requires
TLS and may explicitly generate a task-owned test CA outside Git. Staging accepts
explicit image tags or SHA-256 digests. Production requires reviewed image digests.
Both environments require Gateway HTTPS settings. Use a new tag or digest for a new
release so the installer can detect the change.
Backend/frontend initially have one replica;
backend uses Recreate with retained workspace. This package does not establish HA.

## Upgrade and retained data

Preserve Secrets, all three PVCs, Vault shares/root authorization and state key.
Enable Kubernetes Auth for an existing installation via the backed-up upgrade
workflow. The installer reuses matching reviewer resources and Vault auth
configuration. An existing configuration with a different reviewer or API host
requires operator review.
Stop old writers/frontend, take private PostgreSQL backup and backend/Vault data
archives, validate archives, then run owner setup before starting new runtime.
Restore is operator-controlled with a matching source/image and isolated rehearsal;
there is no automatic database downgrade or reset. New schema changes still follow
[ADR-027](../decisions/ADR-027-postgres-upgrades.md), never edit released migrations.
Idempotent install/upgrade commands do not treat an old completed Job as proof of a
new upgrade. Source/image identity determines a new owner Job receipt.

## Routing and dependencies

HTTPRoute sends the hostname's entire `/` path to frontend; its normal `/api/` proxy
preserves same-origin API access to backend. PostgreSQL/Vault/backend are internal
Services. Gateway must permit the installation namespace; no broadening of shared
allowedRoutes is automatic. TLS/DNS for the Console belongs to the existing Gateway
operator. Gateway/controller, VSO/Fleet, workload RBAC and registry credentials remain
prerequisites for later workload deployment, not proof supplied by Console install.

An optional `vaultUI` object enables the Vault UI. A separate HTTPS hostname in
`vaultUI.hostname` adds an HTTPRoute on the existing Gateway for staging when
Vault serves HTTP internally. Without a hostname, the UI is available via a
private forward. Vault continues to authenticate UI and API requests. Internal
TLS installations can expose the UI through a trusted TLS forward.

The kind overlay is an isolated test installation, never the existing main Console.
A real browser must test the exact Tailscale handoff URL and related APIs. Restore,
restart, same-source reapplication and image update must preserve installation and
catalog identities. Full product checks use separate disposable kind namespaces
and baseline comparison while Product CI remains red.

## Dependencies and boundaries

First-run behavior is [UC-00 SETUP](../../usecase/UC-00/first-run-setup.md), supplied
by the accepted implementation scope of #89 on the task branch; neither feature is
merged to main without user acceptance. Existing personal-machine scripts/manifests
remain supported. Reuse canonical policy files instead of copying policies or
changing another installation's deployment procedure. Operational tooling is not a
new product CLI, planner, API or executor implementation.

## Deployment package consolidation

New Orchestrator installations use the Kustomize package in
`deploy/kubernetes/` and its operator CLI in `scripts/kubernetes/`. The kind
overlay exercises the same installer lifecycle as staging and production.
Docker Compose supports local development and tests.

The retained kind Console is a legacy installation with one PVC. Migrate into
a separately owned namespace and keep the source available for rollback.
Do not relabel the source or change an installer ownership receipt to adopt it.
Capture the source database, Vault storage, state encryption key and backend
workspace while writes are stopped. Rehearse restoration before redirecting
users. Verify the source database installation identifier, domain identities,
Vault references and state decryptability after restoration.

Confirm the Console origin and issuer remain stable for existing OIDC identities.
Verify workloads that reference the previous Vault Service, including a fresh
VSO authentication and reconciliation, before stopping the source Vault. Keep a
single active Console writer at cutover. Retain source volumes and access
configuration until the migrated installation is accepted. If rollback is needed
after writes on the target, reconcile those writes before restoring source
writer access.
