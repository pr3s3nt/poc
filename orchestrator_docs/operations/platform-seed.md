---
id: OPS-PLATFORM-SEED
artifact: operations-runbook
status: current
last_reviewed: 2026-11-06
---

# Local API seed and kind Console

Approved 2026-11-06: install Orchestrator inside `kind-k8s-4f`, expose its Console
through verified Tailscale port 3070, and import platform configuration through
its API from the current checkout. Input YAML and credentials remain local,
outside Git; no input ConfigMap, Secret or seed Job. This replaces the abandoned
direct-service Go draft. Existing browser bootstrap remains separate.

## Installation

`deploy/kind/console.mjs` renders nonsecret manifests in phases `storage`, `setup`,
`console` for a fresh, task-owned namespace `orchestrator-dev-3070`. Check ownership
before applying; do not adopt/reset existing installations. Load the matching
source images into kind first. Apply storage, wait for PostgreSQL/Vault readiness,
apply setup and wait for successful Job completion, then apply console. Applying
manifests alone does not prove readiness. No schema or product behavior changes.
Retained upgrades follow [ADR-027 runbook](postgres-upgrades.md), never a reset.

This personal dev installation uses one RWO PVC on single-node kind, with distinct
subpaths for PostgreSQL, generated credentials, Vault and backend caches. Existing
local bootstrap scripts generate independent owner, Console and State logins and
state key inside the installation. Owner setup mounts owner credentials; backend
mounts only runtime credentials and scoped Vault tokens. Backend cannot mount the
Vault bootstrap/root material. File-backed Vault unseals using retained bootstrap
material: this is local automation, not production HA/KMS. Keep the PVC; back up
PostgreSQL, Vault/bootstrap, credentials, state key and caches together privately.

Use a retained installation-owned frontend Service port-forward. Preserve existing
Serve routes and verify the actual MagicDNS URL with a browser per
[Tailscale handoff](tailscale-handoff.md). Do not remove the existing Compose stack
or its volumes when moving the user-facing port.

## Installation commands

Use the current private kubeconfig and explicitly check the namespace is absent
or has this installation's `app.kubernetes.io/managed-by=poc-local-installer`
label. Do not use these commands to adopt an unrelated namespace. With the
matching images already loaded:

```sh
export KUBECONFIG="$HOME/.local/state/orchestrator-dev-3070/private/kubeconfig"
node deploy/kind/console.mjs storage | kubectl --context kind-k8s-4f apply -f -
kubectl --context kind-k8s-4f -n orchestrator-dev-3070 wait \
  --for=condition=available deployment/postgres deployment/vault --timeout=180s
node deploy/kind/console.mjs setup | kubectl --context kind-k8s-4f apply -f -
kubectl --context kind-k8s-4f -n orchestrator-dev-3070 wait \
  --for=condition=complete job/db-setup --timeout=180s
node deploy/kind/console.mjs console | kubectl --context kind-k8s-4f apply -f -
kubectl --context kind-k8s-4f -n orchestrator-dev-3070 wait \
  --for=condition=available deployment/backend deployment/frontend --timeout=180s
```

The completed owner Job is an initialization receipt, not a mechanism for
upgrading retained data. Reusing it does not authorize a schema/image upgrade.
Rebuild product images from `backend/` and `frontend/` for an intentional source
upgrade, select reviewed tags and follow the database upgrade procedure first.

## API import

`node scripts/bootstrap/seed-api.mjs --config /private/dev.yaml` authenticates
without a browser, reads current configuration, creates missing records, verifies
CD, signs out, reports and exits. Secrets and session cookies stay in memory;
no secret values or raw API/provider diagnostics enter logs or reports. Paths
resolve relative to config; credential files must be owner-only. Use HTTPS, an
explicit loopback forward or an operator-verified Tailscale URL. Do not disable
TLS checking or follow credential-bearing redirects.

Manifest version 1 declares Console URL, username, passwordFile and expected
Organization, optional Git settings, Driver Accounts, Vaults, Types, Git templates,
Definitions and CD. Product Type/Definition JSON is reused inline or from files.
Full commit and content hashes pin Git sources. Named `${accounts.NAME}` and
`${templateSources.NAME}` references resolve in parsed Definition strings only;
product expressions remain intact. Local files are loaded before business writes.

Order: Git, accounts, Vaults, Types, templates, Definitions, CD. A matching identity
is REUSED; different data is CONFLICT. No overwrite, reset, credential rotation or
revision promotion. Credentials are retained, not compared/rotated on reuse.
Earlier successful steps remain after failure. Unknown mutation outcomes require
server read-back before retry; no automatic POST retries. Run one importer per
target; existing product insert/CAS guards continue to apply.

Registration may verify Kubernetes/AWS, perform the usual scoped Vault probe and
CD provider checks. Missing credentials remain prerequisites, never fake READY
records. No Terraform execution, AWS creation, application deployment, controller
installation or shared RBAC replacement is part of seed. Bootstrap success is
not workload acceptance. Application presets were suggested as a follow-up;
no application creation feature is included in this task.

## Restart and retained access

Images for this installation are `orchestrator-backend:seed-80315d31` and
`orchestrator-frontend:seed-80315d31`, built from checkout commit `80315d31` and
loaded into the existing kind node. PostgreSQL/Vault and caches use PVC
`platform-storage`; the source Compose volumes remain separate and unchanged.
Image transport into the kind node used the existing host Docker tooling; code,
manifest generation, API import and kubectl operations run from this checkout.

Local private inputs and kubeconfig are in
`~/.local/state/orchestrator-dev-3070/private/` (directory 0700, files 0600).
The namespace/PVC are retained; no automatic cleanup or backup-expiry task exists.
The local frontend forward uses task-owned tmux session `orch3070-cluster` and
writes `~/.local/state/orchestrator-dev-3070/forward.log`:

```sh
KUBECONFIG="$HOME/.local/state/orchestrator-dev-3070/private/kubeconfig"   kubectl --context kind-k8s-4f -n orchestrator-dev-3070   port-forward --address 127.0.0.1 svc/frontend 13070:8080
```

The separate task-owned session `orch3070-tailnet` runs a socat listener on the
verified Tailscale IPv4 port 3070, forwarding to 127.0.0.1:13070. Recheck the
interface address after host/network changes rather than assuming a historical
IP. The previous task-owned Compose proxy was replaced; the Compose services and
volumes were not stopped. After reboot restart both forwards; Kubernetes itself
restarts the in-cluster Deployments. Restrict private input and evidence access;
never publish their contents.
