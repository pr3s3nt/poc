# Apache Airflow through Console UI — issue #51

Goal: use the platform as a Developer to deploy Apache Airflow from its official
image (`apache/airflow`, released from https://github.com/apache/airflow), and
record what the platform cannot yet do for a multi-component application sharing
one database. Read [e2e/README.md](../../README.md), UC-06/UC-16 and the
[workload authoring guide](../../../orchestrator_docs/operations/workload-authoring.md).
Evidence and limits: [verification record](../../../orchestrator_docs/verification/2026-11-08-airflow.md).

## Topology

Airflow 3.4.0, LocalExecutor, image pinned by digest in `run.mjs`:

| Workload | Template | Containers |
|---|---|---|
| `api-server` | Backend API | `main` (`api-server`, port 8080, HTTP probes on `/api/v2/monitor/health`), init `migrate` (`airflow db migrate` + FAB admin from the image entrypoint) |
| `scheduler` | Xử lý nền | `main` (`scheduler`, also serves task logs), init `wait-migrations` |
| `dag-processor` | Xử lý nền | `main` (`dag-processor`), init `wait-migrations` |
| `triggerer` | Xử lý nền | `main` (`triggerer`), init `wait-migrations` |

All four select one shared `postgres` resource (class `in-cluster`). The SQLAlchemy
URL is built inside the container by `AIRFLOW__DATABASE__SQL_ALCHEMY_CONN_CMD` from
`DB_*` variables that reference the resource outputs, so the password stays a
Secret reference. JWT secret, API secret key, Fernet key and admin password are
Environment secrets. Task logs are fetched from the scheduler by Pod IP
(`HOSTNAME_CALLABLE=get_host_ip_address`). `api-server` has a public endpoint `/`.

## Prerequisites

- Retained Console `orchestrator-dev-3070` on `kind-k8s-4f` with its seeded Git,
  cluster account, Fleet default binding and `Dev Platform Vault`.
- Vault Kubernetes auth for workloads: apply
  `deploy/kind-k8s-4f/04-vault-kubernetes-auth.yaml`, then run
  `deploy/kind-k8s-4f/vault-kubernetes-auth.sh` (see the deploy README).
- An existing Gateway accepting routes from all namespaces, wildcard DNS for the
  root domain and a Tailscale listener reaching the Gateway.
- `npm ci --prefix scripts/bootstrap && npm ci --prefix e2e`, Chromium for Playwright.

`ORCH_TEST_RUN_DIR` is a private directory outside Git with `config.json`
(`publicUrl`, `rootDomain`, `endpointName`, `gatewayName`, `gatewayNamespace`,
`gatewaySectionName`, `clusterKubeconfigFile`, `secretStoreKey`) and
`secrets/{jwt-secret,api-secret-key,fernet-key,admin-password}` (0600). The
Fernet key is URL-safe base64 of 32 bytes. Use the shared Console variables
`ORCH_PLATFORM_URL`, `ORCH_PLATFORM_USERNAME`, `ORCH_PLATFORM_PASSWORD_FILE`.

## Run

From the repository root:

1. `node e2e/scenarios/airflow/run.mjs prepare` creates the Application
   `airflow-kiemthu`, selects the Secret Store and creates the Environment secrets.
2. `node e2e/scenarios/airflow/run.mjs platform` (Platform Engineer) registers
   the `endpoint` Type, the app-scoped endpoint Definition, a Git template source
   pinned to a `main` commit and the app-scoped workload Definition.
3. `node e2e/scenarios/airflow/run.mjs prepare` again authors and saves all four
   workloads through the form, reopening each one.
4. `preview` checks that the batch contains exactly the four workloads; `deploy`
   submits it with one click and waits for the Operation. A recorded attempt is
   never replayed; retry only a confirmed terminal failure with
   `ORCH_RETRY_CONFIRMED=1`, keeping the earlier attempt. `status` is read-only.
5. `node e2e/scenarios/airflow/verify.mjs` checks the cluster read-only, then
   signs in to Airflow through the public URL, triggers `example_bash_operator`
   and reads a task log in the browser.

## Assertions

Drafts saved and reopened before Deploy; exactly one Deploy request per attempt;
Operation SUCCEEDED; four Deployments and PostgreSQL Ready without restarts;
`DB_PASSWORD` is a `secretKeyRef`; HTTPRoute Accepted/ResolvedRefs for the public
hostname; Airflow health reports metadatabase, scheduler, triggerer and
dag-processor healthy; browser sign-in shows the example DAGs; the triggered run
succeeds and its task log is shown in the UI.

## Side effects and cleanup

Creates one Application/namespace and its delivery repository, the app-scoped
Definitions, one Git template source, Environment secrets in Vault, PostgreSQL
with its PVC, VSO objects, four Deployments, one Service and one HTTPRoute.
Vault Kubernetes auth and its reviewer identity are retained platform setup.
The application stays for user review. Remove it through Console Application
deletion and verify namespace/UID absence; do not delete shared Gateway, Fleet or
Vault resources. Logs, screenshots, state and secrets stay in the run directory.
