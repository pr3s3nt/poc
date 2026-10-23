---
id: RUNBOOK-DOCKER-LOCAL
artifact: operations-runbook
status: current
last_reviewed: 2026-10-12
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
Backend and frontend use `pull_policy: build`, so ordinary `docker compose up -d`
checks their source builds even when a local image already exists; unchanged layers
use the Docker build cache. `BACKEND_IMAGE`/`FRONTEND_IMAGE` select output image tags.
The backend Dockerfile uses the requested `mirror.gcr.io` base images and
development-only HTTP/insecure download settings; kubectl checksum validation
remains enabled. The frontend uses the requested mirrored Node/nginx bases and
build-stage development npm TLS settings. PostgreSQL/Vault image overrides still
select runtime images.

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
supervisor renews the three scoped tokens (Connection, application and output-secret) every minute; each token period is 768 hours.
Token files are owned by backend UID 65532 and mounted read-only into backend.
The application token can manage application value/bundle paths and workload
policies/roles; it cannot read Connection credentials. The credential token
cannot read application secrets. The output-secret token may only create and read
`kv/orchestrator/outputs/*` (`deploy/local/outputs-policy.hcl`); it backs the durable
output-secret store the Terraform state backend requires
(`ORCHESTRATOR_OUTPUT_SECRET_STORE=vault`). Root/unseal material stays inaccessible to backend.

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

Restart does not create duplicate ordinary stores or token credentials. A
conflicting endpoint/mount identity or unmarked user registration fails startup
rather than being overwritten. Application token replacement can refresh a
marked managed credential through CAS. T23 removes old workload Vault flags,
legacy store backfill and conversion; use ordinary registration or explicit
platform bootstrap. Retained legacy dev state requires the separately approved
T29 reset; these changes do not authorize restarting or resetting the retained stack.

This configuration is for a personal machine: Vault uses HTTP on the private
Docker network, and its root token and single unseal key are retained in a
separate `vault-bootstrap` volume to support automatic restart. Backend cannot
mount that bootstrap volume. This is local automation, not KMS auto-unseal or a
production Vault deployment. See HashiCorp's
[initialization documentation](https://developer.hashicorp.com/vault/docs/commands/operator/init).

## Backend image tools and AWS mode

The backend image carries `kubectl`, `git`, Terraform 1.9.8 and `score-k8s` 0.15.0,
plus private 0700 directories under `/var/lib/orchestrator` (`recovery`,
`tfroot`, `plugin-cache`, `module-cache`). The Terraform state backend refuses to
start without the Terraform binary, the durable Vault output-secret store and
the recovery directory (`ORCHESTRATOR_TERRAFORM_RECOVERY_DIR`).

The default Compose runs `ORCHESTRATOR_ADAPTERS=kubernetes`. To execute
Definition-driven AWS resources, add the override on every Compose command:

```bash
docker compose -f docker-compose.yml -f docker-compose.aws.yml up -d --build
```

It selects `-adapters aws` with the `acceptance-v2` seed profile and no seeded
catalog. No AWS credential is configured in Compose: register an AWS Connection
in the Console. The override creates real, billable AWS resources when a
Deployment runs; it does not authorize any AWS run by itself.

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
For an existing local kind cluster, use the optional network overlay on every
Compose recreation:

```bash
docker compose -f docker-compose.yml -f deploy/local/compose.kind.yml up -d --build --wait
```

The external `kind` Docker network must already exist. A flattened, single-context
upload can use `https://idp-internal-control-plane:6443` for this machine's
`kind-idp-internal` cluster; preserve its CA and embedded credentials. Keep the
upload private and outside Git. The overlay only attaches backend networking;
it does not install workload Vault auth or deploy Kubernetes resources.
The registration-only Playwright helper verifies or reuses an existing matching
Connection without overwriting it:

```bash
cd frontend
node test/e2e/k8s4f-register.mjs --base-url http://127.0.0.1:3001 --kubeconfig /private/path/kubeconfig --name k8s-4f
```

For internal-k8s execution, select the uploaded Connection in Environment Settings.
ADR-013 supplies its implicit existing-cluster node without user Definition
registration; namespace/PostgreSQL Definitions and workload Vault auth are still required.

The [ordinary-store verification](../verification/2026-10-08-compose-vault-normal-store.md)
covers default source builds, the seeded database store, browser variable/secret
writes against real Vault, token isolation and persistence after container
replacement. The [earlier local verification](../verification/2026-10-07-docker-compose-local.md)
covers Connection registration against a simulated Kubernetes API. Live cluster
authentication and workload delivery are not established by these Compose checks.

## Other kind runners after T23

Updated kind runners share `backend/test/integration/platform-store-lib.sh` for
ordinary store bootstrap. They require `ORCH_CONNECTION_VAULT_ADDR` and an
owner-only `ORCH_CONNECTION_VAULT_TOKEN_FILE` for the separate private credential
store, plus optional `ORCH_CONNECTION_VAULT_MOUNT` (default `kv`). Workload token
files remain bootstrap input only. This is setup guidance, not authorization
to execute live runners or proof that their historical scenarios still pass.

## T28 fresh baseline and isolated PostgreSQL verification

Current builds initialize only an empty PostgreSQL schema. Reboot validates the
current baseline; historical migration ledgers, unmarked tables or incompatible
schema fail before serving with a development-reset-required error. Removing a
ledger or restarting does not upgrade old data. Current complete JSON snapshots
reopen without backfill; missing required data is rejected. See
[canonical baseline](../architecture/database/schema.md#fresh-persistence-baseline-t28--leg-08).
The retained stack must wait for the [T29 backup/reset gate](k8s4f-dev.md#t29-backupreset-gate).

T28's disposable test runner is prepared at
`backend/test/integration/postgres-baseline-local.sh`. After explicit approval:

```bash
bash backend/test/integration/postgres-baseline-local.sh --evidence /tmp/poc-T28-postgres-<unique-run-id>
```

Runner owns a uniquely labelled PostgreSQL 16 container with ephemeral data,
no existing volumes/network, and a dynamically allocated localhost port. It never
reads `.env` or an existing database URL; the suite receives only that server's
URL. It runs uncached full Go tests/build, targeted persistence races and a
synthetic backup/restore smoke, saves exit statuses and cleanup evidence, then
removes only its label-owned container and network. Evidence path must be new and outside Git. This
command needs separate approval for disposable database mutation; it is not a
retained IDP backup/reset, Vault verification or browser scenario.

Baseline boot validation builds its reference schema inside a rolled-back
transaction, so the backend database role needs CREATE privileges on its database.
Existing product schema/data is never repaired and reference schemas are not
retained. T28's [isolated execution](../verification/2026-10-12-T28-fresh-baseline.md)
verified this behavior; retained backups are still unverified until T29.

## EC2 with instance role

On EC2 with Docker Engine and Compose v2, run `./deploy/ec2/start.sh` from the
repository. It merges base Compose, the AWS executor overlay and
`docker-compose.ec2.yml`; all frontend/backend/PostgreSQL/Vault/bootstrap services
run with durable named volumes. Backend/frontend use the existing Dockerfiles.
This developer deployment uses the existing seed login; restrict access with SSH
forwarding, never expose the default demo login publicly.

Require IMDSv2 and allow the backend container to reach metadata (Docker bridge
usually needs response hop limit 2). Do not mount host AWS profiles or set AWS
access keys. Register the expected canonical role in Driver Accounts. All
executor hosts must carry that same pinned role. Block untrusted containers and
workload processes from metadata; sharing an EC2 host shares its role authority.

Optional non-secret settings: EC2_APPLICATION_KEY, EC2_RUN_ID, EC2_OWNER,
EC2_RESOURCE_TTL, FRONTEND_PORT and BACKEND_PORT. Defaults are for development;
set a unique run ID for verification. Forward local port 3001 to the instance's
127.0.0.1:3001, then open `/ui/`. Provisioning remains explicit through the app.
Vault workload address must be reachable by deployed Pods; configure
VAULT_WORKLOAD_ADDRESS before deploying workloads (the Compose hostname alone
is not reachable from a remote EKS cluster).

Stop via `docker compose -f docker-compose.yml -f docker-compose.aws.yml -f
docker-compose.ec2.yml down`. Destroy owned cloud resources through the app before
terminating EC2 or removing durable volumes. Do not add `-v` for retained data.
See [AWS runbook](aws.md) for role permissions, live verification and cleanup.

For automated host provisioning, updates and the restricted ALB trial endpoint,
see [EC2 automation](ec2.md).
