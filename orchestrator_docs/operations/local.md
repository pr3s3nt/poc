---
id: RUNBOOK-LOCAL
artifact: operations-runbook
status: current
last_reviewed: 2026-09-30
---

# Local development with fake adapters

## Build and test

```bash
cd backend
go build ./...
go test ./...
(cd ../frontend && npm ci && npm run typecheck && npm run lint && npm test && npm run build)
```

Kiểm tra onboarding UC-00/UC-01 bằng Chromium thật, fake adapters và JSON state
riêng trong thư mục tạm:

```bash
cd backend
bash test/integration/onboarding-playwright-local.sh
```

Runner sign in, tạo Application chỉ từ Name/Subdomain, kiểm tra đúng
`staging`/`production`, sign out/sign in lại, restart backend và xác nhận
Application vẫn còn. Runner không dùng Docker, Kubernetes hoặc cloud và tự dọn
process/file tạm khi thành công; đặt `ORCH_KEEP_EVIDENCE=1` để giữ log khi cần
điều tra.

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
