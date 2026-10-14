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

## Redis cache (I04-09)

PostgreSQL stays the durable authority. A shared Redis (the same instance for
`backend` and `worker`) caches completed job results:

```text
worker: commit result to PostgreSQL -> write-through SET <key> EX <ttl>
backend: GET <key> -> HIT: serve cached
                      MISS: read the same PostgreSQL row -> SET <key> EX <ttl> (refill)
                      ERROR: serve the PostgreSQL row, report the failure
```

Only completed jobs are cached, so a pending job is always read from PostgreSQL
and is never frozen by an entry. The key is
`acceptance:v1:<app>:<env>:<run>:job:<id>`.

Redis is required, with authentication in-cluster and managed. A cache failure
is never a pass: it is shown in the existing deployment-check table (row
`cache`), `/readyz` returns 503, and `/api/checks` carries safe evidence even
though the durable result is still shown. The worker records the outcome of each
write-through in `jobs.cache_write` (`OK`/`FAILED`); a post-commit failure
keeps the committed job (never reprocessed or duplicated) and keeps the `cache`
check failing. A missing or invalid binding makes the `worker` exit and the
`backend` report `config`.

| Variable | Meaning |
|---|---|
| `REDIS_HOST`, `REDIS_PORT`, `REDIS_USERNAME`, `REDIS_PASSWORD`, `REDIS_TLS` | The `redis` resource outputs (all required; `REDIS_TLS` is `true` or `false`; the password arrives through a Secret) |
| `ACCEPTANCE_CACHE_APP`, `ACCEPTANCE_CACHE_ENV`, `ACCEPTANCE_CACHE_RUN` | Key scope, 1-63 letters, digits, `.`, `_`, `-` (all required) |
| `ACCEPTANCE_CACHE_TTL_SECONDS` | Entry lifetime, 1-3600, default 30 |

With `REDIS_TLS=true` the server certificate chain and the host name (also used
as SNI) are always verified; there is no insecure option. The binaries embed the
public CA roots, so the `FROM scratch` images verify ElastiCache certificates.

`/api/checks` returns, next to the boolean `checks`, a `cache` object:
`configured`, `reachable`, `reason` (`unreachable|auth|tls|timeout|config|error`),
`tls`, `ttlSeconds`, `scope`, `lookups` (counts of `hit`, `miss-refilled`,
`miss-refill-failed`, `error-db-fallback`, `pending`), the 50 most recent
lookups (job id, outcome, reason, time) and `workerWrites` (`ok`, `failed`). Job
responses carry `cache` and `cacheReason`. No value, password or raw error is
included.

Tests need a real PostgreSQL and `redis-server` (they fail, not skip, under
`REQUIRE_POSTGRES=1` or `REQUIRE_REDIS=1`). Run them against a disposable
fixture with `bash backend/test/integration/acceptance-redis-local.sh`.

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
video without reviewing it for local test data. Pass `--human` for a slower
recording that types into every form with a visible cursor; see the kind
runbook for publishing it.
