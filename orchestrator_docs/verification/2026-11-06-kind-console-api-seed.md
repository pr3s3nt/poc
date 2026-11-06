---
id: VERIFY-KIND-CONSOLE-API-SEED-20261106
artifact: verification-report
status: current
last_reviewed: 2026-11-06
---

# In-cluster Console and local API platform seed

## Scope and result

User authorized Orchestrator installation in `kind-k8s-4f`, Tailscale access at
port 3070, and platform seed through that instance's API. No acceptance app or
AWS resource deployment was requested. The earlier app-preset suggestion remains
follow-up discussion. Implemented from checkout branch `features/seed`, product
image source commit `80315d31`. The abandoned direct-Go seed drafts were replaced;
no production Go/React/schema files changed.

The current WSL environment is `Dev-Ubuntu-24.04`; the existing kind Docker node
lives in distro `Ubuntu` on the same Windows machine. Code, manifest rendering,
local CLI, kubectl/API calls and browser verification ran from this checkout.
An isolated minified kubeconfig and kubectl were prepared privately here. Existing
host tooling was used only to transport images into the existing kind node and
read authorized existing dev credential files; no source checkout moved there.

## Retained installation

- Namespace: `orchestrator-dev-3070`, managed-by `poc-local-installer`.
- Deployments: backend, frontend, postgres and vault, all 1/1 Ready.
- Separate owner setup Job `db-setup` completed before runtime startup. Backend
  mounts Console/State URLs and state key, never owner or Vault root material.
- PVC `platform-storage`, 8Gi, UID-backed PV `pvc-73973086-c036-4051-bb18-7e877b38ef4e`.
  PostgreSQL, Vault, generated credentials/state key and caches persist in
  distinct subpaths. Existing Compose volumes/services and other namespaces remain.
- Product images `orchestrator-backend:seed-80315d31` and
  `orchestrator-frontend:seed-80315d31` loaded into the node. Backend image digest
  `sha256:ceb4fea7d52cdd0557cb50c42b2ce2fd39e083315d62e2bb70f818a65b181d7b`;
  frontend `sha256:cbc41450fa401bdf64334b59401f6b5014bf6ec1ae3a637ed8e71d28b83f3904`.

Access checked from the operator machine through the actual Tailscale URL:
[Console resource configuration](http://100.77.224.24:3070/ui/platform/resource-definitions).
Task-owned `orch3070-cluster` forwards frontend Service to loopback 13070;
`orch3070-tailnet` forwards Tailscale IPv4 port 3070 to that listener. This is a
direct Tailscale listener, not a newly configured Serve route. Browser sign-in,
seeded account/type/Definition/Vault/CD pages and same-origin APIs passed. No
claim of verification from a second tailnet device is made. The previous
task-owned Compose proxy was replaced, while Compose itself remains running.

## Seeded data

Private config: `~/.local/state/orchestrator-dev-3070/private/dev.yaml` (valid YAML,
currently JSON formatting). Input/credential files are owner-only, outside Git.
No input file was uploaded as a ConfigMap/Secret or seed Job. Kubernetes generated
its own installation credentials in the PVC; the nonsecret installation scripts
are a ConfigMap. Secret bytes remain required in backend Vault for runtime use.

- Platform Git scope and reusable Git Connection `dev-git` registered.
- Kubernetes Driver Account `k8s-dev` verified against the actual cluster API.
- AWS Driver Account `aws-dev` verified using the existing default profile and
  its declared region. Verification reads identity/capabilities; no AWS resources
  were created, changed or deleted.
- `Dev Platform Vault` is READY for KV-v2 backend operations.
- Seven Resource Types; six Definitions (cluster, namespace, PostgreSQL, Redis,
  persistent storage, workload) with staging and production criteria.
- Git template commit `48035f75af4b3fe7ce1fafd9ebaf41f8b4356d10`, content SHA-256
  `fa4df6f2e54046fe85cefe58c30782663b899cee63ec480047df636be1f55c18`.
  The installed standard source also remains AVAILABLE.
- Git Terraform sources use existing reviewed module commit
  `c78243afdb0173535a02914e5907f9f7ef0b2bec`; the backend inspected each source and
  independently verified exact Definition pins. No Terraform plan/apply ran.
- `dev-fleet` verified READY and selected as platform default. Topology came from
  the backend discovery API rather than hardcoded controller UIDs/configuration.

## Validation and limits

Focused API importer tests cover private-file/control-character guards, schema,
redirect-safe transport, Organization/role checks, redacted failure handling,
unchanged reruns, conflicts, product-placeholder preservation and Definition
mutation detection. Full bootstrap package tests: 78 passed, zero skipped.
Documentation checker and diff whitespace check pass; PlantUML is unavailable,
so its optional local syntax check was skipped. All three installer phases passed
Kubernetes server dry-run after installation. No Go/frontend suites were rerun:
production Go/React/schema are unchanged.

A complete unchanged API rerun preserved record IDs, versions, full returned
metadata and the default binding. Backend was then replaced by a new Pod;
all compared metadata remained identical, demonstrating PVC/cache persistence.
Browser reads passed all five seeded platform pages via the Tailscale URL.
A first exact-text assertion on the Vault page was too strict because the table
combines its name and key; direct page/API inspection and corrected content checks
passed. No product assertion was removed to hide missing data.

The first installation attempt exposed a PostgreSQL subpath ownership issue;
explicit runtime root setup and postgres-owned data directory corrected it,
without resetting the PVC. A failed cross-distro token read was identified by
format checks and repeated privately; the importer now rejects control text in
credential files before business mutations. Initial incomplete import steps were
reused, not duplicated or overwritten.

Vault Kubernetes workload auth is not configured on this new Vault; its Console
explicitly reports that limitation. READY confirms backend KV verification, not
secret-bearing workload deployment. Workload delivery, application creation,
production HA/KMS, backup/restore and remote-device ACL reachability are not
validated by this setup. Do not claim application acceptance from platform seed.

Procedure/restart commands: [runbook](../operations/platform-seed.md).
