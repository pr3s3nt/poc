---
id: IMPLEMENTATION-PACKAGE-LAYOUT
artifact: package-layout
status: current
last_reviewed: 2026-09-30
---

# Backend and Frontend Layout

Package layout follows design ownership; it is not copied from a reference repository.

Go product code nằm trong `backend/`; Orchestrator Web Console nằm trong
`frontend/`. Thư mục `orchestrator_docs/implementation/` chỉ chứa tài liệu thiết
kế/ánh xạ và không phải source tree.

```text
backend/
├── go.mod                                  # module orchestrator
├── cmd/orchestrator/                       # API process entrypoint
├── internal/delivery/http/                 # controllers, request/response mapping, /ui static delivery
├── internal/application/authentication/    # UC-00 sign-in/session service
├── internal/application/application/       # UC-01 self-service Application creation
├── internal/application/deployment/        # UC-06, UC-07, UC-09
├── internal/application/provisioning/      # UC-08
├── internal/domain/application/            # Organization, Application, ExecutionProfile, Connection
├── internal/domain/environment/            # Environment, DeploymentSet, NamespaceIdentity
├── internal/domain/resource/               # Descriptor, Scope, ResourceType/Definition, ActiveResource
├── internal/domain/deployment/             # Deployment, DeploymentPlan, DeploymentResource, WorkloadInstance
├── internal/planning/                      # Score, Delta, graph, match, contract, batches
├── internal/ports/persistence/             # repository and UnitOfWork interfaces
├── internal/ports/execution/               # ResourceExecutor, renderer, deployer, secret store
├── internal/adapters/store/                # Phase 6 state store: in-memory repositories + file snapshot
├── internal/adapters/postgres/             # normalized PostgreSQL repositories and migrations
├── internal/adapters/fake/                 # fake ResourceExecutor/WorkloadDeployer cho walking skeleton
├── internal/adapters/kubernetes/           # resource executor + workload deployer (kubectl transport)
├── internal/adapters/terraform/            # Terraform executor, embedded modules, HCL contract inspector
├── internal/adapters/secrets/              # secret-store implementation
├── internal/seed/                          # Humanitec-style seed catalog cho Phase 6
├── internal/platform/                      # clock, IDs, logging, config
├── test/e2e/                               # HTTP end-to-end tests (fake adapters)
├── test/integration/                       # local browser, PostgreSQL, kind/AWS integration runners
├── test/integration/deployctl/             # drives deployments through the HTTP API
├── test/integration/costreport/            # AWS Pricing API estimate before apply
├── test/conformance/                       # product planner vs the 33 challenge fixtures
└── examples/acceptance-app/                # Go frontend/backend/worker workloads cho E2E verify

frontend/                                   # Orchestrator Web Console
├── src/app/                                # application shell + minimal browser router
├── src/features/auth/                      # UC-00 sign-in page
├── src/features/applications/              # UC-01 application pages
├── src/features/{workloads,deployments}/   # future use-case features, added when scheduled
├── src/shared/{types,ui}/                  # shared UI primitives and client-side types
├── src/styles/                             # design tokens và styles theo concern
├── src/test/                               # Vitest setup
└── test/e2e/                               # Playwright browser flows
```

## Rules

- Go module/package names are short lower-case names; public method names match operation contracts.
- Domain packages import neither delivery, SQL, Terraform nor Kubernetes SDK packages.
- Planning can depend on domain value objects and read-only catalogs, never executor adapters.
- Adapter packages implement interfaces from `internal/ports`.
- No Python runtime/build dependency.
- Frontend và Go backend chỉ chia sẻ JSON contract dưới `/api/v1/`; không import source của nhau.
- Go backend phục vụ production bundle dưới `/ui/`; Vite proxy `/api` trong development.
- Frontend dùng React + TypeScript strict + Vite, Vitest/Testing Library; ưu tiên React state/reducer và minimal router trước khi thêm framework khác.
- Root `frontend/` là web console quản trị. Acceptance application frontend là
  test workload riêng tại `backend/examples/acceptance-app/frontend/`.
- Chỉ thêm package khi increment có executable implementation và tests; không
  tạo placeholder cho deferred management gaps.

## Phase 6 implementation notes

- **State store:** `internal/adapters/store` hiện thực in-memory/JSON local-test
  repositories; `internal/adapters/postgres` hiện thực normalized system of
  record theo canonical schema, migration ledger và PostgreSQL transaction.
  Cả hai dùng chung repository/UnitOfWork ports.
- **Kubernetes transport:** `internal/adapters/kubernetes` gọi `kubectl` với `--context` đã resolve
  thay vì nhúng client-go. Adapter vẫn là implementation của port `execution.ResourceExecutor`
  và `execution.WorkloadDeployer`; lựa chọn này giữ module không có dependency nặng và dùng
  chung một đường đi cho kind lẫn EKS.
- **Terraform transport:** `internal/adapters/terraform` gọi `terraform` CLI với working directory
  và state file riêng theo run ID. Runtime chỉ nhận `source.module` trỏ tới module nhúng
  `vpc`/`eks`/`aurora`; `source.url[@rev][/path]` hiện chỉ được conformance inspector đọc và chưa
  phải execution contract của MVP.
- **Score contract boundary:** API nhận Score document dạng JSON. JSON là tập con của YAML nên
  contract không đổi; orchestrator không cần YAML dependency.
- **Terraform contract inspection:** `internal/adapters/terraform/inspect.go` parse module nhúng bằng
  `hashicorp/hcl/v2` để planning kiểm tra variable/output và ghi source fingerprint (UC-06 BR-09).
  Đây là dependency ngoài duy nhất của orchestrator core; `examples/acceptance-app` thêm `jackc/pgx/v5`.
- **Verification harness:** `deployctl` gửi deployment qua `/api/v1/deployments` thay vì gọi thẳng
  application service, nên kind và AWS verification đi đúng đường Web Console -> HTTP API -> executor.
  Request body không chứa `runId`: process sở hữu run ID qua `-run-id`, API tự điền khi request bỏ trống.
- **Conformance:** `test/conformance` chạy planner sản phẩm trên toàn bộ 33 fixture của planner
  challenge và so sánh Candidate Deployment Set, graph, matching, batches, Terraform contract và
  Active Resource classification. Hai khác biệt có chủ đích được lọc ra và ghi rõ trong package đó:
  workload node không được match/execute (UC-08 chỉ execute resource node) và orchestrator thêm
  namespace/cluster implicit mà challenge không có. 27 accepted fixture được so sánh artifact;
  6 rejected fixture hiện chỉ xác nhận planner từ chối, chưa đối chiếu error code/phase/path.
  Harness so expected Delta (`expected/delta.yaml`) và kiểm `base + delta = candidate`; bundle
  không có array diff hoặc container requests/limits, nên hai vùng này có product tests riêng
  (`internal/planning/jsonpatch`, `internal/planning/delta_test.go`; container resources tại
  `internal/planning/score/resources_test.go`, `internal/planning/resources_test.go`,
  `internal/adapters/kubernetes/resources_test.go`, `internal/application/deployment/resources_test.go`).
  Dependency `gopkg.in/yaml.v3` chỉ dùng ở harness này.
