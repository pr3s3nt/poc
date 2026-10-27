---
id: VERIFY-E2E-ACCEPTANCE-APP-UI-20261027
artifact: verification
status: current
last_reviewed: 2026-10-27
---

# Root e2e acceptance application deploy through the UI

User scope: a staging-like local platform where `k8s-4f` is the kind cluster
kubeconfig, then a Playwright script that deploys the acceptance application
through the Web Console UI. The user asked to stop all running Orchestrator
stacks first; their containers were stopped, not removed, and all volumes kept.

## Environment

- New Compose project `thieu-staging` from commit `0eff925` with
  `deploy/local/compose.kind.yml`, Console `http://localhost:3001`. Real adapters:
  PostgreSQL, Vault, Kubernetes, Terraform, installed score-k8s. Default
  `seed-catalog staging` gave five Types, no account/Definition/Application.
- Target: existing kind cluster `k8s-4f`; account kubeconfig is a private
  minified export with endpoint `https://k8s-4f-control-plane:6443`, outside Git.
- Secret Store: existing `k8s-4f-workload-vault` with its scoped token file.
- Acceptance images `acceptance-{frontend,backend,worker}:thieu-0eff925` built by
  `backend/test/integration/build-images.sh` and loaded into the kind node.

## Runs

| Step | Script | Result |
|---|---|---|
| Account | `e2e/setup/register-internal-account` | Internal `k8s-4f` registered READY; rerun reused it |
| Catalog | `frontend/test/e2e/bootstrap-staging-catalog.mjs` | Created five `staging-k8s-4f-*` entries |
| Store | `e2e/setup/register-secret-store` | `K8S-4F Vault` registered READY; rerun reused it |
| Deploy | `e2e/scenarios/acceptance-app` | PASS, Application `acceptance-r1` |

The first account/store registrations confirmed READY through the creation
response; the reuse checks were then changed to read the on-screen lists and
both reruns passed with the final scripts. The creation paths of the final
scripts were not rerun because that requires removing the registered records.

Deploy run (headless): developer created the Application, selected the store,
saved eight variables and a generated secret (never shown), imported three
Scores, Preview showed three affected workloads, Deploy reported success for
backend, worker and frontend. Through a kubectl port-forward the acceptance page
showed backend connection, environment, secret, database and cache PASS, and a
submitted job reached DONE. Read-only kubectl afterwards: three Deployments 1/1,
one PostgreSQL and one Redis StatefulSet 1/1, both PVCs Bound.

`frontend/test/e2e/k8s4f-register.mjs` no longer works: it targets the retired
Connections page. It was not changed; the new account setup script replaces its
role for root e2e runs.

## Checks

- `cd e2e && npm test`: 6/6 PASS; `node --check` on the new scripts.
- Documentation checker and `git diff --check`.

## Retained state

Application `acceptance-r1` (ID `fdd9b8f2-e65e-42a0-91f6-acb88008022d`), namespace
`app-fdd9b8f2-e65e-42a0-91f6-acb88008022d-staging`, its PostgreSQL/Redis and the
`thieu-staging` stack are kept for the user. No cleanup was run. The stopped
stacks remain stopped with their data.
