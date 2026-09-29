# Acceptance application

Three independent Go workloads the orchestrator deploys to verify UC-06 and UC-08
end to end:

| Workload | Role |
|---|---|
| `frontend` | Serves the sample page, proxies `/api` to the backend Service. |
| `backend` | Owns the `jobs` schema, submits and reads jobs. |
| `worker` | Claims pending jobs, writes results back. |

`backend` and `worker` bind to the same shared Score resource
(`postgres.default#acceptance-db`), so the happy path proves:

```text
frontend -> backend -> PostgreSQL -> worker -> backend -> frontend
```

Database connection settings come from the resource outputs the orchestrator
injects as `PGHOST`, `PGPORT`, `PGDATABASE`, `PGUSER` and `PGPASSWORD`. The
password always arrives through a Kubernetes Secret, never as a plain
Deployment environment value.

The frontend also renders a minimal deployment-check table. Its backend checks
that `ACCEPTANCE_CONFIG` equals `acceptance-config-ok`, that the injected
`ACCEPTANCE_SECRET` hashes to the non-secret `ACCEPTANCE_SECRET_SHA256`, and
that PostgreSQL responds to a ping. The frontend checks it can reach the
backend Service. `/api/checks` returns booleans only, never the secret or
database credentials. Missing bindings show `FAIL`; the existing job form
remains available for a deeper frontend → backend → database → worker check.

Build the images with `test/integration/build-images.sh <tag>`; it produces a
static binary per workload and wraps it in the shared `Dockerfile`.

For a browser-driven deployment and these four basic checks on the existing
`kind-idp-internal` cluster, build the Web Console and run
`bash backend/test/integration/acceptance-playwright-kind.sh` from the repo
root. It creates a run-scoped Application and namespace, then removes only
that namespace after the check. It requires the local Vault/VSO installation
and the scoped backend token file described in the kind runbook. The script
prints the path to `acceptance-full.webm`, a single video of the browser flow
from sign-in through the final check page. Browser actions are slowed and key
screens pause long enough for a person to review them. Evidence stays in an owner-only
temporary directory, including when the browser test fails; do not publish the
video without reviewing it for local test data.
