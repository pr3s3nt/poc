# Orchestrator implementation

Product code for the orchestrator described in `orchestrator_docs/`. Design
documents own the requirements; this tree implements them.

```text
cmd/orchestrator      API process and /ui/ delivery
internal/             domain, planning, application services, ports, adapters
frontend/             Orchestrator Web Console (React + TypeScript + Vite)
examples/acceptance-app/  frontend, backend and worker workloads used for verification
test/                 HTTP end-to-end tests and kind/AWS integration tests
```

## Run the API with fake executors

```bash
go build ./...
go test ./...
(cd frontend && npm ci && npm run build)
go run ./cmd/orchestrator -addr 127.0.0.1:8080 -ui-dir frontend/dist -adapters fake
```

The console is served at `http://127.0.0.1:8080/ui/` and calls `/api/v1/` on the
same origin. `npm run dev` inside `frontend/` proxies `/api` to the same process.

## Internal Kubernetes verification (kind)

```bash
bash test/integration/kind-verify.sh
```

It discovers the local kind cluster, builds and loads the acceptance images,
deploys backend, worker and frontend into a namespace named after the run id,
runs the job flow end to end, verifies the console and deletes everything it
created. It never creates or deletes a kind cluster.

## AWS cloud verification

```bash
bash test/integration/aws-verify.sh
```

It prices the run with the AWS Pricing API, creates temporary ECR repositories,
pushes the acceptance images, provisions VPC, EKS and Aurora Serverless v2 with
Terraform state under a run-specific temporary directory, runs the same job flow
through `kubectl port-forward` and then tears everything down from an EXIT trap.
Cleanup is verified against the AWS API by run-id tag, not by the Terraform exit
code alone.

**This script creates paid AWS resources.** Every resource carries `project`,
`owner`, `environment`, `run-id`, `expires-at` and
`managed-by=orchestrator-verification` tags.

## Boundaries

- Planning is deterministic and side-effect free.
- UC-08 executes resource nodes only; UC-06 applies the workload after outputs exist.
- The Candidate Deployment Set becomes current only after workload readiness.
- Secret values never reach the state store, logs or the UC-09 view.
