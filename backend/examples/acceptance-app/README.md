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

Build the images with `test/integration/build-images.sh <tag>`; it produces a
static binary per workload and wraps it in the shared `Dockerfile`.
