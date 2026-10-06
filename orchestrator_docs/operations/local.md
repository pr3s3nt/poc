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

UC-02/03 catalog and UC-04 validation recording:
`bash backend/test/integration/uc02-04-video-local.sh` from the repository root.
It uses a private headed browser and local backend, records the real URL,
mouse clicks and typing, and creates catalog/Application data via UI. It shows
a supported Definition consumed by Preview without restart, seeded READY
Connections, invalid uploaded-document rejection, single-context inspection and
503 when no credential store is configured. No new Connection becomes READY.
The legacy `--kind` argument affects evidence naming only; it no longer contacts
a cluster. Use `bash backend/test/integration/uc04-kubeconfig-video-local.sh --kind`
for successful live read-only upload registration. Both scripts clean up only
run-owned processes/state; videos remain outside Git.

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

## UC-04 kubeconfig review recording

Run `bash backend/test/integration/uc04-kubeconfig-video-local.sh --kind` for
successful upload registration. The [Playwright script](../../frontend/test/e2e/uc04-kubeconfig-video-local.mjs)
and [shell runner](../../backend/test/integration/uc04-kubeconfig-video-local.sh)
implement the demonstration. Without `--kind`, the script is explicitly
simulated and demonstrates rejection of an unreachable endpoint, not READY. The recording must
show real browser interactions: sign-in, invalid/unsupported input, file upload,
multiple-context selection, inspected cluster/endpoint, Check and save, READY
list and cleared credential input. No route mocks or API-created Connection may
stand in for UI registration. Inspect metadata and READY list are asserted.

Use an isolated local test backend and private temporary directory. Local fake
adapters/memory credential store may demonstrate UI only; label such evidence
as simulated. For live read-only verification, explicitly target only existing
`kind-idp-internal`, write flattened selected kubeconfig directly to a private
file without printing it, and upload that file without showing its contents.
No external AWS/other Kubernetes contexts or persistent Vault policy changes
are needed. Local isolated Vault test service may exercise the durable adapter;
any token is generated test-only, scoped to Connection credentials and consumed
by file path. Stop/remove only run-owned processes/container and credential
files after verification. Do not alter existing platform Vault or workloads.

Private headed Chromium/Xvfb/ffmpeg records a human-paced flow with cursor,
address bar and read pauses, using existing video helpers where possible. Never
record real kubeconfig/credential text or terminal output. Failed/incomplete
runs remain local. Reviewer checks assertions, exit status, full MP4 decode and
settled frames before publication. Upload a new run-unique MP4 to existing
`acceptance-recordings` release, without replacing assets; download and compare
SHA-256 before reporting its URL. Video stays outside Git. This user request
authorizes demo and video upload, not code commit/push or AWS provisioning.

### UC-04 credential store configuration

Upload inspection is available without a store; successful registration requires
one. Configure the durable adapter with `-connection-credential-store vault`,
`-connection-vault-address <Vault API URL>`,
`-connection-vault-token-file <owner-only scoped token path>` and
`-connection-vault-mount <KV v2 mount>` (default `kv`). Equivalent environment
variables are `ORCHESTRATOR_CONNECTION_CREDENTIAL_STORE`,
`ORCHESTRATOR_CONNECTION_VAULT_ADDR` and
`ORCHESTRATOR_CONNECTION_VAULT_TOKEN_FILE`. These are separate from UC-12 flags.
The scoped token needs create/read on the Connection data namespace and delete
on its metadata namespace for rollback; workload policies must not grant access.
Setting flags does not install or modify policies on an existing Vault server.

Default `none` rejects new upload registration with 503 instead of falling back
to host credentials. Explicit `memory` supports local fake adapters with no
persistent JSON/PostgreSQL state only; it is not durable and is refused for real
execution. The new recording runner provisions its own test Vault, so no
existing platform policy change is required for the demo.

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

The bundle digest includes the configured binary and embedded adapter/
provisioner source. After changing either, restart the backend: the same existing
Definition selects the newly installed bundle. Previews/tokens made before the
restart are stale and rejected before provisioning; Pending shows a renderer-only
update even with no Score draft, so Preview again and Deploy. Do not register a
new winning Definition for an upgrade (that older procedure is superseded and
would create an ambiguous tie). The registration-time fingerprint is audit
provenance only. A changed binary without restart, an unavailable bundle ID and
ties are still rejected. The initial API does not update/delete Definitions. The
CLI is not downloaded at render time or installed automatically in deployment
images.

For local CLI tests, put the pinned binary on PATH and run `go test ./...` from
`backend`. CLI-dependent tests skip when absent; synthetic preflight/failure and
pure selection tests still run. Each render uses a private temporary workspace,
cleans it afterward, strips inherited host credentials and permits only
output-only bindings. Live cluster/cloud verification is a separate operation.
