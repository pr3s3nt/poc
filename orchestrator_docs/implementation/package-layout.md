# Planned Backend and Frontend Layout

Package layout follows design ownership; it is not copied from a reference repository.

```text
cmd/orchestrator/                 # API process entrypoint
internal/delivery/http/           # controllers, request/response mapping
internal/application/admin/       # UC-01..UC-04 services
internal/application/preview/     # UC-05
internal/application/deployment/  # UC-06, UC-07, UC-09
internal/application/provisioning/# UC-08
internal/domain/application/
internal/domain/environment/
internal/domain/resource/
internal/domain/deployment/
internal/planning/                # Score, Delta, graph, match, contract, batches
internal/ports/persistence/       # repository and UnitOfWork interfaces
internal/ports/execution/         # ResourceExecutor, renderer, deployer, secret store
internal/adapters/postgres/       # repositories and migrations
internal/adapters/terraform/      # Terraform executor/state integration
internal/adapters/kubernetes/     # resource executor + workload deployer
internal/adapters/aws/            # identity/connection verification helpers
internal/adapters/secrets/        # secret-store implementation
internal/platform/                # clock, IDs, logging, config
test/fixtures/                    # Humanitec-style Go test fixtures
test/integration/                 # PostgreSQL/kind integration tests
examples/acceptance-app/          # Go frontend/backend/worker workloads cho E2E verify
frontend/                         # Orchestrator Web Console (không phải acceptance workload)
frontend/src/app/                 # application shell + minimal browser router
frontend/src/features/            # UI/API/draft/page/component theo use case
frontend/src/shared/api/          # typed same-origin HTTP transport
frontend/src/shared/ui/           # UI primitives không biết use case
frontend/src/styles/              # design tokens và styles theo concern
frontend/src/test/                # Vitest setup và shared fixtures
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
- `frontend/` là web console quản trị. Acceptance application frontend là test workload riêng và không đặt trong cây này.
