# DB connection-string outputs on kind — issue #52

Goal: prove that an application reaches PostgreSQL, Redis, MySQL, MariaDB and
MongoDB using only the connection-string output of each resource (`uri`, MongoDB
`connection`), selected as a resource output in the Console, while the value stays
a Secret reference. It also exercises UC-02 EXT: a Platform Engineer adds optional
outputs to an existing Type in the Console before a Definition provides them.
Contract: [terraform-modules](../../../orchestrator_docs/architecture/contracts/terraform-modules.md#chuỗi-kết-nối--issue-52),
[UC-02 EXT](../../../orchestrator_docs/usecase/UC-02/specification.md#bổ-sung-output-tuỳ-chọn--issue-52).

## Topology

Five workloads (template "Xử lý nền", class `git-dev`), each with one private
resource `db` of class `in-cluster` and one container using the database's own
image as client:

| Workload | Resource | Variables | Check |
|---|---|---|---|
| `pg-check` | postgres | `DATABASE_URL`←`db.uri`, `JDBC_URL`←`db.jdbcUrl` | `psql "$DATABASE_URL"` |
| `redis-check` | redis | `DATABASE_URL`←`db.uri` | `redis-cli -u "$DATABASE_URL" ping` |
| `mysql-check` | mysql | `DATABASE_URL`←`db.uri`, `JDBC_URL`←`db.jdbcUrl` | `mysql` with fields split from the URL |
| `mariadb-check` | mariadb | `DATABASE_URL`←`db.uri`, `JDBC_URL`←`db.jdbcUrl` | `mariadb` with fields split from the URL |
| `mongo-check` | mongodb | `DATABASE_URL`←`db.connection` | `mongosh "$DATABASE_URL"` |

Each container logs `CONNECTION_OK <engine>` (and `JDBC_OK` when the JDBC URL has
the expected form) and keeps running. The `mysql`/`mariadb` CLIs take no URL, so
the script splits the URL into user, password, host, port and database: this still
proves the published string is complete and its credentials work.

## Prerequisites

- A Console built from the feature branch, installed in its own namespace on
  `kind-k8s-4f` and seeded through `scripts/bootstrap/seed-api.mjs` (Git, cluster
  account, workload Vault with Kubernetes auth, Fleet CD, Types, cluster and
  namespace Definitions). The `mariadb` Type is registered without `uri`/`jdbcUrl`,
  as on an installation that predates this change.
- DB images present on the node or pullable; pinned by digest in `run.mjs`.
- `npm ci --prefix scripts/bootstrap && npm ci --prefix e2e`, Chromium for Playwright.

`ORCH_TEST_RUN_DIR` is a private directory outside Git with `config.json`
(`moduleRef` full commit with the modules, `clusterKubeconfigFile`,
`secretStoreKey`). Console variables: `ORCH_PLATFORM_URL`,
`ORCH_PLATFORM_USERNAME`, `ORCH_PLATFORM_PASSWORD_FILE`.

## Run

From the repository root, in order: `platform` (Platform Engineer: add the
optional outputs to `mariadb` through the Resource Types page, then register the
workload Definition and the five DB Definitions pinned to `moduleRef`),
`prepare` (create the Application, select the Secret Store, author and save the
five workloads through the form), `preview`, `deploy` (one batch Deploy), `verify`,
then `cleanup`. A recorded Deploy attempt is never replayed implicitly.

```sh
node e2e/scenarios/db-connection-outputs/run.mjs platform
node e2e/scenarios/db-connection-outputs/run.mjs prepare
node e2e/scenarios/db-connection-outputs/run.mjs deploy
node e2e/scenarios/db-connection-outputs/run.mjs verify
node e2e/scenarios/db-connection-outputs/run.mjs cleanup
```

## Assertions

- The Type gains the outputs through the Console; a Definition mapping them is
  then accepted.
- Preview contains exactly the five workloads; one Deploy request; Operation
  `SUCCEEDED`.
- Every check container logs `CONNECTION_OK`; JDBC URLs have the expected form.
- `DATABASE_URL` is a `secretKeyRef` in every Deployment, never a literal.
- No database password appears in the saved workloads, Preview, Operation or
  Deployment manifests (compared in memory, never printed).

## Side effects and cleanup

`cleanup` first removes the Fleet access Role/RoleBinding this scenario granted
(labelled with the Application), otherwise product deletion keeps the namespace
and reports `CLEANUP_INCOMPLETE`; then it deletes the Application through the
Console (Thử lại resumes a failed deletion) and checks the namespace and PVs are gone.

Deploy creates the application namespace, five database StatefulSets/PVCs, Vault
entries of the run's own Vault, a Fleet GitRepo and the application's delivery
repository on the platform Git account (retained per ADR-033). After `cleanup`, delete the Console's own namespace and its `ClusterRoleBinding`. Evidence stays in `ORCH_TEST_RUN_DIR`.
