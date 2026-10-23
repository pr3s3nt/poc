---
id: RUNBOOK-PREVIEW-KIND
artifact: operations-runbook
status: current
last_reviewed: 2026-10-23
---

# Persistent Preview Console on kind

The `preview-kind` Compose project builds this checkout and uses real Kubernetes
and Terraform adapters, PostgreSQL with separate Console/State logins, encrypted
Terraform state and durable Vault output secrets. It owns independent volumes;
the old memory demo on port 18093 and the retained development stack are preserved.
The installed score-k8s binary is explicitly selected; missing renderer capability
must never be replaced by fake delivery. Module and provider caches survive restarts.

```bash
bash deploy/local/preview-kind.sh up
bash deploy/local/preview-kind.sh status
```

Console defaults to `http://127.0.0.1:18094/ui/`; backend is loopback port 18095.
Set `PREVIEW_BIND_ADDRESS` to this machine's Tailscale IP when starting to expose
the Console only on that interface. The script ignores root `.env`, fixes its
project identity and has no reset or volume deletion command. Stop with
`bash deploy/local/preview-kind.sh stop`; use the same binding when starting again.
Local seeded login is `platform-engineer` / `test-password`.

Startup installs no cluster resources. The catalog starts empty: register the
Internal Driver Account through the Console with a private, flattened kubeconfig
whose endpoint is reachable from the backend on the kind Docker network. Register
Types, an existing-cluster Definition, namespace Terraform Definition and a
workload rendering template with explicit references to that cluster/namespace.
Placement comes from these Definitions; there is no Environment Connection fallback.
Use the packaged modules or verified Git sources from the
[module runbook](terraform-module-sources.md).

For secret-bearing workloads register a workload-reachable Vault with Kubernetes
auth/VSO configured for the selected cluster, then explicitly select it in the
Environment. The bundled platform Vault is a credential/output store; its Compose
hostname alone is not a Pod-reachable workload store. Existing workload Vault may
be registered using its scoped token privately; never print credentials or state.

Create an Application, save Score drafts, Preview, inspect the graph/code, then
Deploy explicitly. Validate real Kubernetes readiness and inventory, update/reuse,
refresh/restart persistence and the exact workload's applied manifest. Verification
uses new owned Applications/namespaces only; cleanup preserves preexisting cluster,
accounts, operators, stores and workloads. No AWS deployment is part of this setup.

This runbook describes the intended operational configuration. Actual execution
results and any remaining gaps belong in the dated verification report.

## Prepared installation on this machine

The [23 October execution](../verification/2026-10-23-preview-kind-live.md)
completed the procedure on `kind-k8s-4f`. Use
<http://100.77.224.24:18094/ui/> and the retained
[sample Application](http://100.77.224.24:18094/ui/applications/7de8dc65-afcf-4808-9809-340c0225c79a).
Its deployed diagnostic frontend is at <http://100.77.224.24:18096/>.
The port 18093 Console remains the old fake-adapter review demo with independent data.

The Internal account is `kind-k8s-4f`. Definitions `kind-cluster`,
`kind-namespace`, `kind-postgres`, `kind-redis` and template `kind-workload`
provide the existing cluster, per-Environment namespace and in-cluster services
for new Applications too. PostgreSQL/Redis use class `in-cluster`.
For a new secret-bearing Environment explicitly select **Kind Workload Vault**
(`kind-workload-vault`) in Settings; the **Platform Vault** Compose endpoint
does not provide the prepared Pod-reachable workload authentication.
Supply an image the cluster can pull, or load a local image into `k8s-4f`;
the Console deploys images and does not build application source code.
The displayed example.com names do not configure DNS/TLS; the sample's port
18096 is its retained Service forward.

To rebuild/restart the Console on the same Tailscale interface:

```bash
PREVIEW_BIND_ADDRESS=100.77.224.24 bash deploy/local/preview-kind.sh up
```

Docker services have persistent volumes and restart policies. The sample forward
lives in the task-owned `preview-kind-app` tmux session. After a host reboot,
reopen it in a terminal if the tmux session did not survive:

```bash
kubectl --context kind-k8s-4f \
  -n app-7de8dc65-afcf-4808-9809-340c0225c79a-staging \
  port-forward --address 100.77.224.24 svc/frontend 18096:8080
```

The committed live browser scripts require explicit URLs, private input paths
and new evidence directories. Bootstrap resume is only for its own undeployed
Application; it never resets data or overwrites existing catalog entries.
`preview-kind-check.mjs` can observe an existing sample, optionally execute its
known image update, and create/remove a separate owned cleanup smoke Application.
On failure retain its identity/evidence and inspect product state before any
retry; there is no broad host-side namespace deletion fallback.
