---
id: RUNBOOK-LOCAL
artifact: operations-runbook
status: current
last_reviewed: 2026-10-06
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

## UC-05 human-paced Score Preview recording

After building the Web Console, run from the repository root:

```bash
bash backend/test/integration/uc05-video-local.sh
```

This reuses the headed full-window recording contract above, but performs all
Application/workload setup through the UI on camera. Seeded local test accounts
and catalog are the only initial fixtures. It demonstrates invalid input,
successful standalone Preview, all planning artifacts, scope-change clearing,
no-change update and absence of a saved preview draft. Runtime adapters are
fake; it does not prove Kubernetes/cloud execution. MP4, timestamps, logs and
sampled frames remain outside source. Output directories must be new; never
overwrite historical evidence.

## UC-07 human-paced update/remove recording

After building the Web Console, run from the repository root:

```bash
bash backend/test/integration/uc07-video-local.sh
```

The [runner](../../backend/test/integration/uc07-video-local.sh) invokes
[Playwright](../../frontend/test/e2e/uc07-video-local.mjs) on a fresh local test
store. Application and workload creation, Preview/Deploy, update, Delete
cancel/confirm, Undo and history/detail navigation use only UI input on camera.
The native confirmation is answered by real X11 keyboard input. Removal shows
that an unused resource becomes unreferenced, not destroyed, while another
workload remains. The full-window recording includes the address bar, visible
pointer and sequential typing. Fake runtime adapters mean this is not proof of
live Kubernetes/AWS effects. Reviewer artifacts remain outside source; the
runner cleans up only its own processes and temporary JSON state.

## Run

UC-02..04 human-paced registration recording:
`bash backend/test/integration/uc02-04-video-local.sh` from the repository root.
It uses a private headed browser and fake/local backend, records the real URL,
mouse clicks and sequential typing, and creates catalog/Application data via UI.
It shows a supported Definition consumed by Preview without restarting, seeded
READY Connections and invalid Connection input, not successful live verification.
Artifacts stay under `/tmp`; only owned processes and temporary state are cleaned.
For successful live kind connection verification, use the explicit `--kind`
mode described in the [kind recording runbook](kind.md#human-review-recording-for-the-current-workload-editor); this mode contacts the existing cluster read-only.

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

## Optional Definition-selected score-k8s rendering

Install a trusted score-k8s **0.15.0** binary on the backend host. Enable it with
an explicit path; startup rejects an unavailable binary or another version:

```bash
cd backend
go run ./cmd/orchestrator -adapters fake -score-k8s /absolute/path/to/score-k8s
```

On Web Console → Resource definitions, choose **score-k8s workload renderer**.
The form selects Type `workload`, profile `internal-k8s`, and bundle
`score-k8s-internal-v1`; add the Application/Environment matching criteria before
registering. Empty criteria is a wildcard affecting every matching internal
Application. Database and namespace Definitions remain independent.

Preview Score shows Definition/renderer version without running the CLI. Saved
workload changes use the normal Preview → Deploy flow. Rendering-only Definition
changes also appear as pending updates. No matching renderer Definition means
the built-in Kubernetes renderer. An explicitly selected unavailable renderer
fails; there is no fallback after selection.

The bundle fingerprint includes the configured binary and embedded adapter/
provisioner source. Changing either invalidates an existing Definition's bundle
fingerprint; restart the backend and register a new Definition with criteria
that win matching, then Preview again. Ties are rejected. This initial API does
not update/delete Definitions. The CLI is not downloaded at render time or
installed automatically in deployment images.

For local CLI tests, put the pinned binary on PATH and run `go test ./...` from
`backend`. CLI-dependent tests skip when absent; synthetic preflight/failure and
pure selection tests still run. Each render uses a private temporary workspace,
cleans it afterward, strips inherited host credentials and permits only
output-only bindings. Live cluster/cloud verification is a separate operation.
