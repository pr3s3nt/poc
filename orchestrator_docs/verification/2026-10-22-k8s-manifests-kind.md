---
id: VERIFY-K8S-MANIFESTS-KIND-2026-10-22
artifact: verification-record
status: current
last_reviewed: 2026-10-22
---

# Kubernetes manifest smoke test on existing kind

User requested manifests and a trial deployment on existing kind because the
destination shared Kubernetes cluster is inaccessible. Source checkout was
`444f10f` on branch `deploy_k8s`; only deployment files and documentation changed.
Runbook: [Kubernetes bundle](../../deploy/k8s/README.md).

## Environment and ownership

- Explicit context `kind-idp-internal`, existing single-node kind cluster
  `idp-internal`, Kubernetes v1.36.1, amd64.
- Created namespace `orchestrator-demo`, UID
  `8608df23-f78d-41b3-93d5-6457842f08de`, with managed-by `poc-k8s-manifests`.
- New PostgreSQL, Vault, recovery PVCs and four generated Secrets; no existing
  database, Vault release, controller or shared application was modified.
- Built current backend/frontend Dockerfiles as `orchestrator-backend:deploy-k8s`
  and `orchestrator-frontend:deploy-k8s`. PostgreSQL `16-alpine`, Vault `1.21`.

## Results

| Check | Result |
|---|---|
| Backend/frontend image builds | PASS; frontend build includes TypeScript checks |
| Embedded shell script syntax | PASS |
| Documentation checker, Python/YAML syntax and whitespace checks | PASS |
| Initial and repeated server-side dry run of base manifests | PASS |
| Four Deployments Ready, three PVCs Bound | PASS |
| Frontend `/ui/` HTML through Service port-forward | PASS |
| API health through frontend `/api/` proxy | PASS |
| Developer sign-in and authenticated session | PASS |
| Create application `Kubernetes manifest smoke` | PASS |
| Application read after replacing all four component Pods | PASS |
| Scoped application Vault token write/read | PASS |
| Vault value read after Pod replacement, then probe metadata deletion | PASS |
| Repeated credential provisioning keeps existing credentials | PASS |
| Backend UID 65532, credential files 0400, owner credential absent | PASS |
| Recovery directory permission 0700 | PASS |
| Database setup on second boot verifies baseline without changes | PASS |

The first attempt exposed PostgreSQL Secret file ownership: root-owned 0400 files
were unreadable after the image dropped to the postgres user. Fixed the manifest
with a private memory-backed copy owned by the image's postgres UID. Removed only
this run's freshly created PostgreSQL Deployment/PVC, retained its generated
Secrets, and repeated initialization successfully against a fresh test database.
The final install was then restarted and persistence checks passed.

`kind load docker-image` initially failed on incomplete multi-platform image
metadata (`content digest ... not found`). Imported the same images using
`docker image save --platform linux/amd64` piped into the existing node's
`ctr images import --platform linux/amd64`; the runbook records this fallback.

## Limits and retained resources

The kind cluster has Traefik but no Gateway API resource types. Consequently,
`httproute.example.yaml` was prepared but not applied or validated against a live
Gateway. External HTTPS, certificate coverage, listener namespace permissions,
registry access and destination storage remain unverified. The destination
cluster was not accessed and DNS was not changed.

No application source changed; full product test suites were not rerun. This is
an installation/API smoke test, not browser E2E, workload provisioning, Kubernetes
Connection onboarding, workload Vault auth/VSO, cloud verification or production
readiness certification.

The test namespace, four ready Deployments, four Services, three PVCs and four
Secrets are retained for inspection. The created smoke application is retained;
the standalone Vault probe was deleted. A local frontend port-forward listens on
127.0.0.1:3301 during this session; the runbook includes the command to restart it.
Build/import logs and the temporary API smoke helper are outside Git under
`/tmp/deploy-k8s-*`; they contain no credential values.
