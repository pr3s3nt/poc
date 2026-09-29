---
id: RUNBOOK-SELF-HOST-KIND
artifact: operations-runbook
status: current
last_reviewed: 2026-09-29
---

# Self-hosted Orchestrator on kind

This procedure runs the Orchestrator as two container workloads deployed by the
Orchestrator itself, then uses that in-cluster instance to deploy the
diagnostic acceptance app. It is a kind demonstration with the limits recorded
as IMP-014 in [deviations](../implementation/deviations.md): the in-cluster API
receives the host's admin kubeconfig as a UC-12 secret and keeps state in
memory, so a Pod restart loses Applications and drafts.

## Images

| Image | Build | Runtime |
|---|---|---|
| `orchestrator-backend` | `docker build -t orchestrator-backend:<tag> backend` | Go API with checksum-verified kubectl `v1.36.1`, non-root, port 8080 |
| `orchestrator-frontend` | `docker build -t orchestrator-frontend:<tag> frontend` | nginx serving `/ui/` and proxying `/api/` to `BACKEND_URL`, port 8080 |

The API entrypoint maps `ORCHESTRATOR_*` variables to flags. It decodes
`ORCHESTRATOR_KUBECONFIG_B64` and writes `ORCHESTRATOR_VAULT_TOKEN` to
owner-only files under `/tmp/orchestrator`, then unsets both values. Supported
settings: `ORCHESTRATOR_KUBE_CONTEXT`, `ORCHESTRATOR_CLUSTER`,
`ORCHESTRATOR_VAULT_ADDR`, `ORCHESTRATOR_VAULT_AGENT_ADDR`,
`ORCHESTRATOR_VAULT_DELIVERY`, `ORCHESTRATOR_ADAPTERS` (default `kubernetes`)
and `ORCHESTRATOR_LISTEN_ADDR` (default `0.0.0.0:8080`).

## Recorded run

Prerequisites: the kind cluster, Vault/VSO and scoped token from
[Vault on kind](vault-kind.md), Traefik, a built Web Console, Playwright
dependencies, and `Xvfb` and `ffmpeg` on the host.

```bash
cd frontend && npm ci && npm run build
cd ..
bash backend/test/integration/self-host-playwright-kind.sh
```

The script builds the Orchestrator and acceptance images with the run ID and
loads them into kind. It then starts a host Orchestrator and opens a headed
Chromium on an Xvfb display, and ffmpeg records the full window, including
tabs and the address bar. Every form is operated with a visible cursor and
typed input; the kubeconfig and Vault token are pasted into masked secret
fields. The recorded flow:

1. The host Orchestrator creates Application `Orchestrator` (subdomain
   `orchestrator`), its variables and secrets, and workloads `backend` and
   `frontend` with public path `/`, then previews and deploys them.
2. A second tab opens `http://staging.orchestrator.example.com/`. Chromium maps
   `*.example.com` to a Traefik port-forward. The in-cluster Orchestrator
   deploys the acceptance app through the same form flow.
3. A third tab opens the acceptance app's public route, reviews the four
   `PASS` checks and submits one job.

The script prints `video=<evidence>/self-host.mp4`. It deletes the acceptance
app namespace and keeps the self-hosted Orchestrator namespace running. A rerun
refuses to start while `staging.orchestrator.example.com` is still routed. To
rerun, delete that namespace after checking its `orchestrator.io/application`
label, or set `ORCH_SELF_SUBDOMAIN`. Review the video before publishing, then
upload it as an asset of the private `acceptance-recordings` pre-release:

```bash
gh release upload acceptance-recordings <reviewed-video>.mp4
```

To open the kept instance later:

```bash
kubectl --context kind-idp-internal -n traefik port-forward svc/traefik 18380:80
chromium --host-resolver-rules='MAP *.example.com 127.0.0.1:18380' http://staging.orchestrator.example.com/
```
