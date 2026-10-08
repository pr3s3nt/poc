---
id: RUNBOOK-DOCKER-LOCAL
artifact: operations-runbook
status: current
last_reviewed: 2026-10-08
---

# Personal-machine Docker Compose

The root [Compose file](../../docker-compose.yml) runs PostgreSQL, the backend,
the Web Console and persistent Vault. It configures the UC-04
[Connection credential store](../architecture/connection-credentials.md).
The local Vault also supplies an explicitly seeded `platform-vault` Secret Store
through explicit local registration bootstrap. It is an ordinary verified store,
using the same validation and credential storage as stores registered in the UI. Separate scoped tokens
serve Connection credentials and application secrets. Its metadata is persisted
in PostgreSQL; token and secret values stay in Vault/private token volumes.
New Environments still select a store explicitly in Settings. Compose enables
local secret storage; Kubernetes workload delivery additionally needs a reachable
Vault address and Kubernetes auth configured for the selected cluster.

## Start

Use Docker Engine with Docker Compose v2 or later. From the repository root:

```bash
docker compose up -d
docker compose ps
```

The default backend/frontend images build from current repository source and
PostgreSQL/Vault use public images, so no private Harbor login or source override
is required. Override image references with `BACKEND_IMAGE`, `FRONTEND_IMAGE`,
`POSTGRES_IMAGE` or `VAULT_IMAGE` in the shell or an ignored root `.env` file.
Use `docker compose up -d --build` after source changes to rebuild existing images.

Open <http://localhost:3001/ui/>. The backend is at <http://localhost:8080>.
Override the host ports with `FRONTEND_PORT` and `BACKEND_PORT` when needed.
PostgreSQL and Vault are reachable only inside the Compose network.
The local profile seeds `platform-engineer` and `developer` accounts with password
`test-password`. Existing database contents remain authoritative.

The [source override](../../deploy/local/compose.source.yml) remains compatible
with existing commands, but is optional:

```bash
docker compose -f docker-compose.yml -f deploy/local/compose.source.yml up -d --build --wait
```

Port 3001 avoids another application already listening on port 3000 on the
verification machine. Override the port if needed:

```bash
FRONTEND_PORT=3002 docker compose up -d --build --wait
```

Then open <http://localhost:3002/ui/>. Use the same override and port on subsequent
Compose commands to retain this image/port selection.

Vault starts with file storage, initializes once, unseals automatically, enables
KV v2 at `kv` and creates separate scoped periodic tokens. The backend waits for
PostgreSQL and Vault health checks. No manual token copy is needed. The Vault
supervisor renews both tokens every minute; each token period is 768 hours.
Token files are owned by backend UID 65532 and mounted read-only into backend.
The application token can manage application value/bundle paths and workload
policies/roles; it cannot read Connection credentials. The credential token
cannot read application secrets. Root/unseal material stays inaccessible to backend.

Compose sets `ORCHESTRATOR_PLATFORM_VAULT_BOOTSTRAP=true` with
`ORCHESTRATOR_PLATFORM_VAULT_ADDR`, `ORCHESTRATOR_PLATFORM_VAULT_WORKLOAD_ADDR`
and `ORCHESTRATOR_PLATFORM_VAULT_TOKEN_FILE`. The last variable points to the
private application token used during registration/bootstrap, not a runtime
fallback. Ordinary store resolution reads the persisted credential reference
from the configured platform credential store. Compose therefore does not set
legacy `ORCHESTRATOR_VAULT_ADDR` or `ORCHESTRATOR_VAULT_TOKEN_FILE`.
`ORCHESTRATOR_CONNECTION_VAULT_TOKEN_FILE` remains the separate credential-store
authentication configuration; users adding other Vaults do not add backend flags.

Sign in, open Platform → Secret stores to see `Platform Vault` (key
`platform-vault`), then select it in an Environment's Settings before adding a
secret. Bootstrap verifies KV v2, backend permissions and probe cleanup before recording
READY; this does not verify workload Kubernetes auth. The store token is persisted
privately in the platform credential store, and runtime resolves its opaque
reference. The application token file is bootstrap input only. Kubernetes auth is
checked before secret-dependent Deploy. Set `VAULT_WORKLOAD_ADDRESS` to an address
reachable from the selected cluster before the initial seed when preparing live
workload delivery; Compose does not install or mutate that cluster.

Existing Compose legacy records are upgraded in place after verification, retaining
store identity, selected Environments and secret references. Restart does not create
duplicate stores or token credentials. A conflicting endpoint/mount identity fails
startup rather than overwriting another registration. Application token replacement
can refresh the managed credential on backend startup. Legacy Vault flags remain
available for non-Compose compatibility, but are not enabled by this Compose profile.

This configuration is for a personal machine: Vault uses HTTP on the private
Docker network, and its root token and single unseal key are retained in a
separate `vault-bootstrap` volume to support automatic restart. Backend cannot
mount that bootstrap volume. This is local automation, not KMS auto-unseal or a
production Vault deployment. See HashiCorp's
[initialization documentation](https://developer.hashicorp.com/vault/docs/commands/operator/init).

## Stop, restart and troubleshoot

```bash
docker compose down
docker compose up -d
docker compose logs --tail=100 vault backend frontend
```

Named volumes retain PostgreSQL data, Vault data, bootstrap material and the
scoped tokens through ordinary `down`/`up` and container replacement.
Back up the database, Vault data/bootstrap and both token volumes together.
`docker compose down -v` deletes them and
loses applications, accounts, Connections and credentials.

If the scoped token expires after more than 768 hours offline or is revoked,
Vault bootstrap creates a replacement. Recreate backend so it reads that token:

```bash
docker compose up -d --force-recreate backend
```

Uploaded kubeconfig must contain an endpoint reachable from the backend
container; `127.0.0.1` in that endpoint refers to the container itself. External
credential commands/files in uploaded kubeconfig are rejected. Registration
uses read-only cluster verification; deployment is an explicit subsequent action.
To use an uploaded Connection for execution, register a matching `existing-cluster`
Resource Definition as described in the shared credential design.

The [ordinary-store verification](../verification/2026-10-08-compose-vault-normal-store.md)
covers default source builds, the seeded database store, browser variable/secret
writes against real Vault, token isolation and persistence after container
replacement. The [earlier local verification](../verification/2026-10-07-docker-compose-local.md)
covers Connection registration against a simulated Kubernetes API. Live cluster
authentication and workload delivery are not established by these Compose checks.
