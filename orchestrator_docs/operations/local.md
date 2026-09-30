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

## UC-09 human-paced review recording

The local review recording must capture a headed Chromium window, including
the real address bar, visible pointer movement, clicks and sequential keyboard
input. Pause at important views so a reviewer can read status, failure reason,
graph/batches, workload digest and redacted outputs. Do not replace demonstrated
UI actions with hidden API requests or a fabricated URL overlay.

Deployment-history fixtures may be prepared before recording with fake runtime
adapters and a private temporary JSON store. Identify that setup explicitly in
the execution evidence: this is a UI/read-model demonstration, not proof of
Kubernetes/cloud workload execution. Recorded credentials are only fixed local
test accounts, with password inputs masked.

Use an isolated Xvfb display and capture it with ffmpeg. Stop owned backend,
browser, recorder and display processes after the run, but retain the video
and reviewer frames outside tracked source. Review sampled video frames before
publishing a uniquely named asset; never overwrite historical recordings.

After building the Web Console, run:

```bash
bash backend/test/integration/uc09-video-local.sh
```

The [runner](../../backend/test/integration/uc09-video-local.sh) invokes
[Playwright](../../frontend/test/e2e/uc09-video-local.mjs), retains an MP4,
timestamps and sampled frames in a temporary evidence directory, and does not
touch a cluster. Xvfb, ffmpeg/ffprobe and Chromium are required; native X11
keyboard input uses xdotool. If it is absent, the runner extracts the Linux
distribution package into its private work directory without a system install.

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
