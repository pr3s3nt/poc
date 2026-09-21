---
id: RUNBOOK-LOCAL
artifact: operations-runbook
status: current
last_reviewed: 2026-09-21
---

# Local development with fake adapters

## Build and test

```bash
cd backend
go build ./...
go test ./...
(cd ../frontend && npm ci && npm run typecheck && npm run lint && npm test && npm run build)
```

## Run

Fake mode không chạm cluster hoặc cloud account:

```bash
cd backend
go run ./cmd/orchestrator \
  -addr 127.0.0.1:8080 \
  -ui-dir ../frontend/dist \
  -adapters fake \
  -region us-east-1 \
  -account-id 000000000000 \
  -run-id local-demo
```

Mở `http://127.0.0.1:8080/ui/`. API cùng origin nằm dưới `/api/v1/`.

## Expected baseline

- Applications `acceptance` và `acceptance-cloud` được seed khi cấu hình phù hợp.
- Deploy `backend`, `worker`, rồi `frontend` để dùng shared database fixture.
- Deployment details hiển thị graph/batches/resources/workloads và redacted
  password.

State không có `-state` sẽ mất khi process dừng. Với `-state <file>`, JSON
snapshot không chứa secret value; file vẫn là local runtime data và không được
commit.
