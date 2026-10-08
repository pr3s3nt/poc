---
id: VERIFY-20261008-COMPOSE-VAULT-BOOTSTRAP
artifact: verification-evidence
status: evidence
last_reviewed: 2026-10-08
---

# Compose Vault bootstrap verification

Scope: [local Compose runbook](../operations/docker-local.md). Claude cloud
implemented the three runtime files in commit `4acc8ec`; coordinator reviewed
and verified locally. No Go/frontend product source changed. Existing user
containers and volumes were preserved; no Kubernetes or AWS mutation occurred.

## Environment and results

The isolated `compose-vault-review` project used empty environment-file overrides,
loopback backend port 18080 and frontend port 13001. Plain `up -d --wait` built
`orchestrator-backend:local` and `orchestrator-frontend:local` from source when
absent; PostgreSQL used `postgres:16-alpine` and Vault `hashicorp/vault:1.21`.
All four services became healthy. Frontend image build included TypeScript checks.

| Check | Result |
|---|---|
| Base and optional source-override Compose configuration | Passed |
| POSIX shell and Bash syntax | Passed |
| Automatic database seed | Exactly one `platform-vault`, READY, legacy=true, verification legacy=true/verified=false |
| Backend private token files | Readable at UID 65532; root/bootstrap volume unavailable |
| Scoped-token isolation | Distinct tokens, UID 65532/mode 0400; application token denied Connection paths and Connection token denied application paths |
| Scoped CRUD | Real Vault write/read/metadata-delete passed for each allowed scope |
| Platform Engineer browser login/list | Seeded Platform Vault (legacy) visible as READY |
| Developer browser flow | Application created, explicit Staging store selected, ordinary variable and synthetic secret saved; reload retained state |
| Secret confidentiality | Secret not rendered after save, actual Vault value matched synthetic input, PostgreSQL dump contained no secret plaintext |
| Database representation | Ordinary variable persisted as value; Secret persisted as opaque KV reference and store key |
| Full Compose down/up without volume deletion | All services healthy again; secret readable and store count still one |

Browser actions used Playwright with the shared human-paced helpers and real
HTTP services, without fake infrastructure adapters. Disposable private synthetic
input and orchestration logs lived under `/tmp/poc-compose-vault-review`, outside
Git. No token or secret value was printed into evidence.

The default `orchestrator-local` stack was then started at loopback ports 8080
and 3001 with its existing PostgreSQL/Vault volumes preserved. All four services
were healthy, the frontend API proxy returned `status=ok`, and the existing
database contained the seeded READY `platform-vault`. This default stack was
left running for the user. Only the isolated review project and its five owned
volumes were removed after verification.

## Limits

The seed deliberately reuses the existing configured-platform compatibility
record; it does not claim successful Kubernetes-auth verification. This run
exercised local secret storage, not workload deployment, VSO installation,
external Vault exposure or production Vault security/HA. No schema or product
behavior changed, so full Go/frontend unit suites were not repeated. Documentation
checker and whitespace validation are required before handoff.
