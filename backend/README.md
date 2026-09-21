# Orchestrator backend

Product code for the orchestrator described in `orchestrator_docs/`. The design
documents own the requirements; this tree implements them.

```text
cmd/orchestrator          API process and /ui/ delivery
internal/                 domain, planning, application services, ports, adapters
examples/acceptance-app/  frontend, backend and worker workloads used for verification
test/                     e2e, conformance and kind/AWS integration tests
../frontend/              Orchestrator Web Console (React + TypeScript + Vite)
```

## 1. Prerequisites

| Tool | Needed for |
|---|---|
| Go 1.25+ | building and testing everything |
| Node 22+ | the Web Console bundle |
| Docker | building the acceptance images |
| kubectl | the internal-k8s and aws-eks execution profiles |
| kind | the internal-k8s verification |
| terraform 1.5+, aws CLI v2 | the aws-eks execution profile |

## 2. Build and test

```bash
cd backend
go build ./...
go test ./...                 # unit, e2e and the 33 planner-challenge fixtures
(cd ../frontend && npm ci && npm run build)
```

`../frontend/dist` must exist before the Go process can serve the console.

## 3. Run locally with fake executors

Nothing touches a cluster or a cloud account in this mode. Use it to explore the
console and the API.

```bash
go run ./cmd/orchestrator -addr 127.0.0.1:8080 -ui-dir ../frontend/dist -adapters fake \
  -region us-east-1 -account-id 000000000000 -run-id local-demo
```

Open `http://127.0.0.1:8080/ui/`. Bind to `0.0.0.0` instead of `127.0.0.1` to
reach it from a Windows host over WSL2.

Two Applications are registered: `acceptance` (internal-k8s) and, when `-region`
is set, `acceptance-cloud` (aws-eks). Deploy `backend`, then `worker`, then
`frontend`: the database is a shared resource the first two both bind to.

## 4. Deploy to a real kind cluster

One command does everything and cleans up afterwards:

```bash
bash test/integration/kind-verify.sh
```

To keep the environment running and drive it from the console instead:

```bash
RUN=dev1
test/integration/build-images.sh "$RUN"
for w in frontend backend worker; do
  kind load docker-image "acceptance-$w:$RUN" --name idp-internal
done

go run ./cmd/orchestrator -addr 127.0.0.1:8080 -ui-dir ../frontend/dist \
  -adapters kubernetes -kube-context kind-idp-internal -cluster idp-internal \
  -namespace "acceptance-$RUN" -state "/tmp/orchestrator-$RUN.json" \
  -frontend-image "acceptance-frontend:$RUN" \
  -backend-image "acceptance-backend:$RUN" \
  -worker-image "acceptance-worker:$RUN"
```

Deploy `backend`, `worker` and `frontend` from the console, then reach the
acceptance application:

```bash
kubectl --context kind-idp-internal port-forward -n "acceptance-$RUN" svc/frontend 8081:8080
# http://127.0.0.1:8081 submits a job; the worker processes it through PostgreSQL
```

Clean up when finished:

```bash
kubectl --context kind-idp-internal delete namespace "acceptance-$RUN"
```

## 5. Deploy to AWS

**This creates paid resources.** The script prices the run, provisions VPC, EKS
and Aurora Serverless v2, deploys the three workloads through the HTTP API,
verifies the job flow and the console, then tears everything down from an EXIT
trap and proves the account is clean by run-id tag.

```bash
bash test/integration/aws-verify.sh
```

Running it by hand instead leaves **no automatic cleanup**:

```bash
RUN="aws-$(date -u +%Y%m%d%H%M%S)"
go run ./cmd/orchestrator -addr 127.0.0.1:8080 -ui-dir ../frontend/dist \
  -adapters aws -region us-east-1 -account-id <account> -run-id "$RUN" \
  -cloud-namespace "acceptance-$RUN" -terraform-root "/tmp/tf-$RUN" \
  -state "/tmp/orchestrator-$RUN.json" \
  -frontend-image <ecr>/acceptance-frontend:<tag> \
  -backend-image <ecr>/acceptance-backend:<tag> \
  -worker-image <ecr>/acceptance-worker:<tag>

# afterwards, always:
RUN_ID="$RUN" TERRAFORM_ROOT="/tmp/tf-$RUN" AWS_REGION=us-east-1 \
  bash test/integration/aws-cleanup.sh
```

The EKS node pulls from ECR, so the images must be pushed to a registry the
cluster can read; `aws-verify.sh` shows the repository creation and push it uses.

## 6. Deploy your own workload

The API is the contract. `POST /api/v1/deployments`:

```json
{
  "applicationKey": "acceptance",
  "environmentKey": "dev",
  "workloadId": "api",
  "actor": "me",
  "score": {
    "apiVersion": "score.dev/v1b1",
    "metadata": { "name": "api" },
    "containers": {
      "main": {
        "image": "ghcr.io/acme/api:v1",
        "variables": { "PGHOST": "${resources.db.host}" }
      }
    },
    "service": { "ports": { "http": { "port": 8080, "targetPort": 8080 } } },
    "resources": {
      "db": {
        "type": "postgres",
        "id": "acceptance-db",
        "params": { "database": "acceptance", "username": "app" }
      }
    }
  }
}
```

Rules that bite first:

- `workloadId` must equal `metadata.name`.
- A resource with `id` is shared inside the Environment; without `id` it is
  private to the workload.
- `params` keys must exist in the Resource Type input contract.
- `${resources.<name>.<output>}` only resolves outputs the Resource Type declares.
- Deploying the same workload twice needs `scoreBefore` in the request: the
  planner refuses to plan from a snapshot it cannot prove. The console has no
  field for it yet, so a redeploy goes through the API.

Other endpoints: `GET /api/v1/applications`, `GET /api/v1/score-samples`,
`GET /api/v1/deployments`, `GET /api/v1/deployments/{id}`.

## 7. State

Without `-state` everything lives in memory and disappears on restart. With
`-state <file>` the process writes a JSON snapshot after every transaction and
reloads it on start. Secret values are never written to that file; only opaque
references are, and the deployment view redacts them.

## 8. What is not implemented yet

- Screens for UC-01..UC-05 and UC-07: Applications, Environments, Resource Types,
  Resource Definitions, Connections, Preview and update/remove. The catalog comes
  from `internal/seed` at startup.
- A PostgreSQL adapter for the orchestrator's own state store.
- A durable Terraform state backend; state lives under `-terraform-root`.
- Cloud infrastructure names still carry the run id, so a new run id provisions
  new infrastructure.
- Rollback, retry, RBAC, audit and secret lifecycle stay out of scope by design.

## 9. Boundaries the code keeps

- Planning is deterministic and side-effect free.
- UC-08 executes resource nodes only; UC-06 applies the workload after outputs exist.
- The Candidate Deployment Set becomes current only after workload readiness.
- Secret values never reach the state store, the logs or the UC-09 view.
