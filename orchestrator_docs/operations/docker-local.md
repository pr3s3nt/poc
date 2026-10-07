---
id: RUNBOOK-DOCKER-LOCAL
artifact: operations-runbook
status: current
last_reviewed: 2026-10-07
---

# Personal-machine Docker Compose

The root [Compose file](../../docker-compose.yml) runs PostgreSQL, the backend,
the Web Console and persistent Vault. It configures the UC-04
[Connection credential store](../architecture/connection-credentials.md).
It does not configure UC-12 workload secret delivery or install a Kubernetes cluster.

## Start

Use Docker Engine with Docker Compose v2 or later. From the repository root:

```bash
docker compose up -d
docker compose ps
```

The default backend/frontend images use the user's Harbor registry; authenticate
with `docker login harbor.stg.exampledevops.com` first if required. The PostgreSQL
image uses `harbor.stg.srvdevops.com`. Images must contain the current Connection
upload implementation and Docker entrypoint. Override image references with
`BACKEND_IMAGE`, `FRONTEND_IMAGE`, `POSTGRES_IMAGE` or `VAULT_IMAGE` in the shell
or an ignored root `.env` file. Public PostgreSQL can use `POSTGRES_IMAGE=postgres:16`.

Open <http://localhost:3000/ui/>. The backend is at <http://localhost:8080>.
Override the host ports with `FRONTEND_PORT` and `BACKEND_PORT` when needed.
PostgreSQL and Vault are reachable only inside the Compose network.
The local profile seeds `platform-engineer` and `developer` accounts with password
`test-password`. Existing database contents remain authoritative.

If Harbor cannot be reached, build current source with the
[source override](../../deploy/local/compose.source.yml):

```bash
docker compose -f docker-compose.yml -f deploy/local/compose.source.yml up -d --build --wait
```

On the verification machine port 3000 belongs to another application, so use:

```bash
FRONTEND_PORT=3001 docker compose -f docker-compose.yml -f deploy/local/compose.source.yml up -d --build --wait
```

Then open <http://localhost:3001/ui/>. Use the same override and port on subsequent
Compose commands to retain this image/port selection.

Vault starts with file storage, initializes once, unseals automatically, enables
KV v2 at `kv` and creates a scoped periodic token. The backend waits for
PostgreSQL and Vault health checks. No manual token copy is needed. The Vault
supervisor renews the token every minute; the token period is 768 hours.
The token file is owned by backend UID 65532 and mounted read-only into backend.

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
Connection token through ordinary `down`/`up` and container replacement.
Back up all four volumes together. `docker compose down -v` deletes them and
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

The [local verification](../verification/2026-10-07-docker-compose-local.md)
covers source-built images and a simulated Kubernetes API. The user's private
Harbor images remain unverified because registry access timed out.
