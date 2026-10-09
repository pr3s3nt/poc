---
id: RUNBOOK-KIND
artifact: operations-runbook
status: current
last_reviewed: 2026-10-09
---

# kind verification

## Safety boundary

- Script dùng cluster kind có sẵn; không tạo hoặc xóa cluster.
- Mọi object phải nằm trong namespace riêng có run ID hoặc mang label run ID.
- Không đổi current Kubernetes context và không sửa workload ngoài namespace
  test.
- Cleanup verification là một phần bắt buộc của run.

## Automated verification

The UC-04 host-context verifier can be checked without creating objects:

```bash
cd backend
ORCH_KIND_VERIFY=1 go test ./internal/adapters/kubernetes -run TestKindConnectionVerifierReadOnly -count=1 -v
```

It checks the named context, API, endpoint and create permissions using
`kubectl auth can-i`; it does not create a namespace or workload.

```bash
cd backend
bash test/integration/kind-verify.sh
```

Script build ba acceptance images, load vào cluster, chạy orchestrator qua HTTP,
deploy backend/worker/frontend, kiểm job flow và Web Console, sau đó xóa
namespace trong cleanup trap.

The separate Backstage check uses the preloaded
`ghcr.io/backstage/backstage:1.53.1` amd64 image, the existing Vault/VSO
installation and Traefik. It creates an Application, stores URL settings via
UC-12, deploys a PostgreSQL-backed Backstage workload through UC-16, checks
the public HTML/health/guest API, then deletes only its run-scoped namespace:

```bash
bash backend/test/integration/backstage-kind-verify.sh
```

For this local check, guest sign-in is explicitly allowed in the container;
do not reuse that setting for production. The script needs the scoped Vault
backend token file documented in [Vault on kind](vault-kind.md). It neither
rebuilds nor mirrors the Backstage image to Harbor.

The browser-driven acceptance check deploys the repository's two diagnostic
workloads through the Web Console, including UC-12 variable/secret references
and a PostgreSQL resource. Playwright then opens the deployed app and requires
`PASS` for backend connectivity, environment, secret and database. It uses the
existing kind cluster, Vault/VSO and a run-scoped namespace:

```bash
cd frontend && npm run build
cd ..
bash backend/test/integration/acceptance-playwright-kind.sh
```

The script needs the scoped Vault backend token file documented in
[Vault on kind](vault-kind.md), stores temporary evidence outside the repository,
and deletes its own namespace after execution. It prints a `video=...` path to
`acceptance-full.webm`, covering the browser flow from sign-in to the final
diagnostic page. Actions run with a short delay and the login, configuration,
Score import, Preview, Deploy result and final diagnostic screens include
review pauses. The video is retained on success and browser-test failure;
review it for local test data before sharing. It does not test cloud delivery
or the worker job flow.

For a recording a person can follow, pass `--human`. The script then runs
`frontend/test/e2e/acceptance-kind-human.mjs`: it enters both workloads through
the workload form instead of Score import, types every value key by key, moves
a visible cursor with a click ripple, and submits one job on the deployed app.
The headed recording includes tabs/address bar and takes approximately 4–6
minutes plus image build/preflight. It writes `acceptance-review.mp4`, phase
marks, probe/decode results, settled frames and cleanup evidence. Publish
reviewed recordings as assets of the `acceptance-recordings` GitHub pre-release
instead of committing them:

```bash
bash backend/test/integration/acceptance-playwright-kind.sh --human
gh release upload acceptance-recordings <reviewed-run-specific-video>.mp4
```

Definition-selected score-k8s rendering (needs score-k8s 0.15.0 on `PATH`) is
verified with a headed, recorded browser run: a Platform Engineer registers a
renderer Definition scoped to the run's Application/staging, Preview shows its
provenance and Deploy applies through the real adapters. The runner logs each
score-k8s CLI call through a wrapper and cleans only its namespace:

```bash
bash backend/test/integration/template-engine-playwright-kind.sh
```

Public Ingress check (run-scoped, separate from the acceptance workload):

```bash
bash backend/test/integration/public-ingress-kind-verify.sh
```

This check creates one Application/namespace, deploys a BusyBox HTTP workload,
selects its Service port as public, forwards the existing Traefik ClusterIP
Service to local port 18380, sends HTTP with the desired `Host` header, removes
the workload and verifies that the Ingress was removed. The script deletes
only its labeled namespace. This is not permanent Windows-host exposure:
configure controller port mapping and DNS/hosts separately. No TLS is set.

For a workload that remains deployed, a temporary local browser path is:

```bash
kubectl --context kind-idp-internal -n traefik port-forward svc/traefik 8081:80
```

While this command runs, send requests with the configured Host header, for
example `curl -H 'Host: staging.myapp.example.com' http://127.0.0.1:8081/`.
For a browser, map that hostname to loopback in the host's `hosts` file and
open `http://staging.myapp.example.com:8081/`. WSL2 localhost forwarding to
Windows depends on the host configuration and was not verified by this run.
Stop the port-forward to close the temporary path.

## Required evidence

- Context/cluster/version đã resolve.
- Namespace và run ID.
- PostgreSQL StatefulSet/Service/PVC ready.
- Backend, worker và frontend ready.
- Live Deployment container resources khớp Score theo UC-06 BR-11
  (`container-resources.txt`, assertion trong `TestKindInternalVerification`).
- Frontend → backend → PostgreSQL → worker → backend → frontend job flow pass.
- Password chỉ đi qua Kubernetes Secret và bị redact ở API/UI.
- Sau cleanup không còn object mang run ID; cluster vẫn tồn tại.

Ghi kết quả thành dated record mới dưới `orchestrator_docs/verification/`.

## Human review recording for the current workload editor

The `--human` acceptance path must use the current UC-16 form: Application
variable/secret bindings are selected by checkbox, same-name mappings require
no retyping, and an explicit name override is used only for aliases. Resource
outputs and workload Services use Other sources. Every shown product mutation
uses the browser UI; API response observation and Kubernetes readiness/cleanup
checks may assert results, but do not replace UI actions or seed fixtures.

Record a headed Chromium window, including real tabs/address bar, visible
cursor and deliberate typing (about 80–120 ms per character), with readable
pauses after Settings, workload save, Preview, Deploy and application checks.
Native select popups must be visible and operated through keyboard input in
headed mode. Use a private display and retain recordings on failure. Capture
phase timestamps and validate complete video decoding, dimensions/duration
and settled frames before publication. Login and secret inputs stay masked;
do not record terminal output or credential/state files. Publish only reviewed
video files with new run-specific names to `acceptance-recordings`; do not
replace old assets or commit generated recordings.

The run uses the existing `kind-idp-internal` cluster and existing Vault/VSO,
builds/loads acceptance images, deploys only run-owned namespaces, checks the
diagnostic app through the browser and verifies cleanup. Explicitly clear
unintended database/credential-file environment defaults in temporary backend
runners. Do not change the current kube context, persistent platform services
or existing workloads. The scoped Vault token is consumed only by the backend
and is never shown in logs/video.

A separate Platform Engineer recording is an independent scenario, executed
after the Developer deployment recording. Its identity and target environment
must be confirmed before a cloud/account-dependent step. Local seeded test
accounts do not prove production RBAC. Record actual validation/registration
results and accurately distinguish live cluster verification from fake-adapter
checks. Update dated verification evidence with outcomes, reviewed asset links
and cleanup observations; a video is not evidence of success unless assertions
passed.

For Platform Engineer catalog validation, use the local `uc02-04` runner:
Resource Type/Definition validation and no-restart Developer Preview remain
covered there. For Connection upload, use the separate `uc04-kubeconfig` runner:
invalid document and unsupported auth rejection, safe context selection,
read-only kind verification, scoped secret storage and READY list. All use the
seeded `platform-engineer` identity and temporary backend. No AWS onboarding
or additional workload deployment is performed by either Platform scenario.

Commands for the review recordings:

```bash
cd frontend && npm run build
cd ..
bash backend/test/integration/acceptance-playwright-kind.sh --human
bash backend/test/integration/uc02-04-video-local.sh
bash backend/test/integration/uc04-kubeconfig-video-local.sh --kind
```

The UC-04 runner writes `uc04-kubeconfig-kind-review.mp4`. It uploads a private
flattened selected-context kubeconfig, verifies API/RBAC read-only and stores
credential in a run-owned isolated Vault test container. Executor adapters
remain fake, so this recording creates no cluster objects and does not replace
platform Vault policies. The local `uc02-04-video-local.sh` runner retains
catalog/Preview and validation coverage; its legacy `--kind` option no longer
performs live connection registration.

Namespace cleanup
in the Developer run does not remove Vault value revisions, per-deployment ACL
policies/auth roles or acceptance images loaded into kind/local Docker. Record
these existing lifecycle limits with each execution result. Confirm namespace
absence via a successful Kubernetes response, never by interpreting a generic
API failure as NotFound. Vault forwarding uses a run-owned dynamic local port.

## Environment-selected uploaded Connection recording

For UC-01 editable Environment Settings selection through a credential-backed Kubernetes Connection, run:

```bash
cd frontend && npm run build
cd ..
bash backend/test/integration/application-connection-kind-video.sh
```

The [runner](../../backend/test/integration/application-connection-kind-video.sh)
uses real Kubernetes executors on the existing `kind-idp-internal` cluster.
The [human browser flow](../../frontend/test/e2e/application-connection-kind-human.mjs)
registers two uploaded logical Connections via Platform Engineer UI; the
internal cluster node is bound implicitly without per-cluster Definition registration, then signs in as Developer, creates an unconfigured Application, then sets the nondefault
Connection in staging Environment Settings and separately sets production, verifies that an empty
Environment can be rebound, configures workloads through forms,
Previews/Deploys, and opens the deployed diagnostic app. Connection/
Application/configuration mutations are UI actions; API reads and Kubernetes
readiness/cleanup assertions are observers.

An isolated run-owned Vault container stores the Connection credential; UC-12
configuration uses the existing scoped platform Vault token by file path.
The backend's private host-context credential is deliberately invalid, while
the uploaded selected credential is valid, so successful workload execution
proves it did not fall back to host authentication. Both logical Connections
refer to the same physical kind cluster; this does not demonstrate deployment
between two distinct clusters. Never display private kubeconfig/token files.

Use the full-window recording/review/publication rules above. The runner keeps
MP4, phase marks, settled frames, diagnostic/readiness assertions and cleanup
proof outside Git under `/tmp/poc-environment-connection-review/` by default
(`ORCH_RESULT_DIR` overrides the base directory). It deletes only its owned
namespaces, temporary processes, private files and credential Vault container;
platform Vault value revisions/policies and loaded images have the existing
lifecycle limits. Review completed successful evidence before uploading a
uniquely named MP4 to `acceptance-recordings`; never replace older assets.

Replay follows ADR-012: a new app starts unconfigured, each Environment keeps its own
editable binding after refresh/restart, and changing a destination with existing
runtime requires a transition. Production is bound but not deployed; both
Connections use the same physical kind cluster. The
[set-once recording](../verification/2026-10-07-environment-connection-kind.md)
is historical evidence of the superseded ADR-011 behavior.

## Environment store/target transition verification

Build the console and run the dedicated human recorder locally:

```bash
cd frontend && npm run build
cd ..
bash backend/test/integration/environment-stores-kind-video.sh
```

The runner creates two workload Vault dev stores with scoped backend tokens and
distinct Kubernetes auth mounts, plus a separate platform credential Vault.
Platform Engineer registers both workload stores through the UI; Developer selects
the Connection and store per Environment, deploys a job, switches stores and rolls
out VSO references, rejects a stale two-tab Preview, migrates PostgreSQL, checks
destination HTTP/data and refresh/restart persistence, then explicitly cleans the
source generation. Both logical Connections target the same physical kind cluster.
The default `video` mode is the acceptance flow; `ORCH_RUN_MODE=probe` checks setup
only and supplies no human-flow evidence.

Artifacts stay under `/tmp/poc-environment-stores-review/live/<run-id>/` by default
(`ORCH_RESULT_DIR` overrides the base). A successful default run includes an H.264 MP4 at
1440×900, at least 420 seconds and 30 phase marks, full decode/frame
checks, safe persisted/API projections and cleanup proof. Review successful
evidence before uploading its unique video name to `acceptance-recordings`.
Cleanup proves source ownership by the original run identity and destination
ownership by the exact generation namespace and deterministic transition identity;
both also require the Application/Environment and managed-by labels.

Use existing kind-idp-internal explicitly, run-owned Vault KV v2 stores/auth paths
and namespaces isolated by target generation. No AWS/other context mutation.
Verify Vault backend capability AND workload auth/VSO access. Create a nonempty
PostgreSQL record before migration, stop source writers, backup/restore and check
record content after destination rollout/Ingress cutover. Keep source generation
until explicit cleanup; inspect owned labels before deletion. Video/artifacts
must exclude tokens, Secret bytes and dump contents. Cleanup run-owned stores,
private data and owned namespaces; retain operator releases/context unchanged.

## ADR-013 internal cluster onboarding

New internal deployments select the uploaded Environment Connection directly;
no per-cluster Definition is required. Historical recordings above may show
manual Definition registration. Their evidence remains historical. The implicit
node and system Definition follow [ADR-013](../architecture/decisions/ADR-013-implicit-existing-cluster.md).

To run the existing human Playwright flow with an exact staging Connection name:

```bash
ORCH_E2E_STAGING_CONNECTION_NAME=k8s-4f bash backend/test/integration/application-connection-kind-video.sh
```

This starts an isolated backend with real Kubernetes adapters, a run-owned
Connection credential Vault and a separate workload Vault on the kind network.
The workload store is registered through the API during setup using a scoped
token, then selected in the UI. Its Kubernetes auth uses a run-owned reviewer
ServiceAccount/ClusterRoleBinding; these and both Vault containers are removed
on exit. The persistent `vault-uc12` token/policy are not used or changed.
It does not deploy through the persistent Docker
Console. Register a retained Connection there separately using the
[Compose procedure](docker-local.md). Both uploads target the same existing kind
cluster; the production logical Connection is bound but never deployed.
The host-context credential is deliberately invalid, proving execution uses the
uploaded credential. The runner checks FE/BE availability, PostgreSQL StatefulSet
readiness/PVC binding, live diagnostic/job behavior and the builtin cluster
Definition. Cleanup requires managed-by, Application, Environment and the exact
deployment run ID observed from Preview before namespace deletion, then verifies
absence using a successful API lookup. Existing Vault/VSO releases are retained.
