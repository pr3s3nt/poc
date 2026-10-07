---
id: VERIFY-20261007-DOCKER-COMPOSE-LOCAL
artifact: verification-evidence
status: evidence
last_reviewed: 2026-10-07
---

# Personal-machine Compose verification

Scope: [local Compose runbook](../operations/docker-local.md), PostgreSQL,
backend, frontend and durable UC-04 Vault credential storage. No existing
Kubernetes cluster or cloud resources were modified.

## Environment and startup

Docker Engine 29.5.2; Docker Compose v5.1.4. Port 3000 was already occupied by
an existing agentgateway container; it was left running. Verification used
frontend port 3001 and backend port 8080, bound to loopback.

Pulling the supplied Harbor images failed with a TLS handshake timeout at
`harbor.stg.exampledevops.com`. Current backend/frontend source was built into
`orchestrator-backend:compose-check` and `orchestrator-frontend:compose-check`.
PostgreSQL used the existing public `postgres:16-alpine` image. Vault used
`hashicorp/vault:1.21`, which ran Vault v1.21.4.

```bash
FRONTEND_PORT=3001 docker compose -f docker-compose.yml -f deploy/local/compose.source.yml up -d --build --wait
```

## Observed results

| Check | Result |
|---|---|
| Backend and frontend image builds | Passed; frontend build includes TypeScript checking |
| All four services healthy | Passed |
| Backend `/api/v1/healthz` | HTTP 200, `status=ok` |
| Frontend proxy `/api/v1/healthz` | HTTP 200, `status=ok` |
| Backend UID can read its token file | Passed |
| Scoped Vault token renewal | Passed |
| Headless Chromium login as `platform-engineer` | Passed |
| Browser navigation to Connections | Passed; no page errors |
| Paste and inspect synthetic kubeconfig in browser | HTTP 200 |
| Check and save in browser | HTTP 201; Connection `READY` |
| PostgreSQL registration row | `READY`, authentication type `KUBECONFIG` |
| Full Compose `down` then `up --wait` | Passed; all services healthy again |
| Credential after container replacement | Readable from Vault using scoped token |
| Login and Connection listing after replacement | Passed; test Connection retained `READY` |
| Cleanup | Removed only the synthetic Connection row and its Vault metadata/versions |

The kubeconfig contained a synthetic token and pointed to a temporary Node HTTP
Kubernetes API simulation bound to the owned Compose network gateway. It served
version/discovery endpoints and five SelfSubjectAccessReview responses. The
real backend kubectl binary performed verification against this simulation;
this proves the browser → backend → Vault/PostgreSQL registration path, not real
cluster authentication, RBAC or workload deployment. The temporary server was
stopped after the check. No real credential values were printed.

The four Compose services and their volumes were left running for user review.
Existing unrelated containers remained running. No commit or push was performed.

## Limits

Private Harbor image `v1` availability and behavior remain unverified. Live
Kubernetes/AWS deployment and UC-12 workload secret delivery were outside scope.
No product Go or frontend code changed, so full unit suites were not rerun;
validation focused on image builds, real container runtime, browser behavior,
credential persistence and Compose/shell/documentation checks.
