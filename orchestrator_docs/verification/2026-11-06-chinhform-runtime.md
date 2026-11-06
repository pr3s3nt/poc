---
id: VER-CHINHFORM-RUNTIME-20261106
artifact: verification-record
status: current
last_reviewed: 2026-11-06
---

# Custom workload runtime form — source, API and real Preview

Branch `features/chinhform`, extending the independently committed class dropdown
`dd6e9c6f`. Scope: [GW-06/07](../architecture/contracts/generic-workload.md) and
[WC-16..21](../usecase/UC-16/workload-catalog-specification.md). No coding agents
were delegated. The user authorized completing the agreed features and keeping
a running Console for manual testing.

## Implemented and checked

Custom and preset forms support managed PVC mounts, named emptyDir volumes
(disk/RAM and size limit), hostPath node sources, literal nonsecret ConfigMap files and scoped UC-12
Secret files. Shared mounts target app/init/native-sidecar containers. Lifecycle
and ordered init declarations survive Save/edit/applied reconstruction. File
Secret creation can read UTF-8 files up to 64 KiB; only UC-12 receives contents.
Workload snapshots and Preview retain references, not secret bytes. ConfigMap
content changes create a new content-addressed reference in the Pod template.

Reviewed companion gaps are implemented: per-container ports, Service protocol,
zero Deployment replicas, advanced annotations and HTTP/TCP/exec probes with
explicit timing. Source capability admission happens before provisioning;
semantic guards verify order, modes, mounts, approved sources and file Secret
projection. Preview now shows init/sidecar containers and volume references,
including nested CronJob Pods, while redacting env/ConfigMap values and exec
probe command text.

## Browser/API result: PASS

The runner [workload-runtime-form](../../e2e/scenarios/workload-runtime-form/README.md)
uses the shared Console login helper with real PE/Developer sessions, PostgreSQL
persistence and the dev Vault. It creates an owned app and app-scoped Definition;
new prepared source revisions are selected explicitly for that app only.
It saves through the form, reopens/resaves without changing the desired document,
and requests Preview. Parsed manifest assertions verify one app container, two
ordered init containers (one native sidecar), all five volume sources, shared
mounts, Secret mode/reference and redacted ConfigMap data. Secret bytes are checked
absent from both workload API responses and Preview. No Deploy was requested.
The app/draft survived a backend replacement during this run.

- App: `2933d17d-2f84-4c19-b20a-55baf5c662dd`.
- Workload: `app`, Environment `staging`, class `chinhform`.
- Scoped Definition: `chinhform-1793897271006`.
- Current source: `33e5ff09-46b8-4e80-83d3-e5f65fd9a8e0`.
- Latest check: 2026-11-06 00:28:53 Asia/Bangkok (2026-11-05 17:28:53 UTC).
- [Verified Console editor](http://desktop-bdjp2rq-1.tail342836.ts.net:5191/ui/applications/2933d17d-2f84-4c19-b20a-55baf5c662dd/environments/staging/workloads/app).

Private receipt, screenshots, generated secret input, source/backend rollback
configuration and response evidence remain outside Git at
`~/.local/state/chinhform-preview/`. This directory must stay private. The retained
app/draft/Secret/Definition are for the user's manual testing; cleanup must use
product APIs against these exact identities. No cluster namespace/PVC/Pod or
cloud resource was created, applied or deleted by this task.

## Automated validation

Pass: Go tests for domain/environment, templatesource, workloadspec, planner,
workloadconfig, templatedriver, deploymentreview, Kubernetes adapter, HTTP API,
store and renderengine; `go build ./...`; frontend typecheck/lint/build and 100
focused editor/env/class/runtime/locale tests; documentation checker,
PlantUML generation and whitespace checks. Renderer tests cover Deployment,
StatefulSet and CronJob with all five mount sources on every container role,
plus changed ConfigMap revisions and explicit probe intent.

Existing fixture failures discovered in the directly affected packages were
corrected without weakening product guards: planning fixtures now prepare a real
pinned template source; zero replicas has a positive round-trip assertion and
negative replicas remains rejected; the standard example reflects omission of
an empty env list. Browser locators explicitly wait for store metadata and select
a storage Type; select controls have stable accessible names.

Broader consumer checks also ran. Known unrelated failures remain in deployment
(inventory kind discovery/removal fixtures), pending (legacy installed-source
fixtures), templatesources (`cmd provisioner`) and bootstrap (`native renderer`).
Representative failures in all four groups reproduce unchanged on an isolated
`dd6e9c6f` checkout; logs are `/tmp/chinhform-consumer-tests.log` and
`/tmp/chinhform-consumer-baseline.log`. They were not skipped or converted into
exceptions. Lint reports the existing PlatformSettingsPage ref-cleanup warning;
existing async class tests emit act warnings; Vite reports the existing chunk-size
warning. No claim is made that the whole repository suite is green.

## Runtime and limits

Frontend runs from this checkout in tmux session `chinhform-console`, port 5191,
proxied by the existing Tailscale Serve configuration. API traffic goes to the
real `endpoint-dev-backend-1` on port 8098. Its local image
`orchestrator-backend:chinhform-preview` replaces only the binary on the existing
dev runtime image, preserving all existing DB/credential/cache/recovery volumes
and networks. No schema upgrade/reset was needed. Restart backend with
`docker compose -p endpoint-dev -f ~/.local/state/chinhform-preview/backend.compose.json up -d --no-deps backend`.
A private matching rollback file retains the old image/configuration. Frontend
restart command is recorded in the same private directory's `start-frontend.sh`.
Do not close the session or remove Serve while handing this link to the user.

Other app Definitions were not repinned. As with earlier runtime upgrades,
old executable-pinned sources need an explicit Definition/source upgrade before
their next Preview; running cluster workloads and their current artifacts were
not changed. End-to-end Kubernetes execution, storage retention across a real Pod
restart, live Job completion with a sidecar and secret rotation on the cluster
remain external verification, not evidence claimed by this authoring/Preview run.
Native sidecars require a target that supports Kubernetes native sidecars.

## HostPath continuation — 2026-11-06

The user approved hostPath and completion in the same checkout. WC-21 extends
the canonical spec/realization, GW-06 contract, existing JSONB/domain design,
sequence/class diagrams and traceability. Named sources now admit a clean Linux
node path and every Kubernetes hostPath type, preserving omitted type and explicit
read-write intent. Form defaults are Directory and read-only new mounts; one
short source note explains node scope and cluster admission. No filesystem
read/provision/delete or workload Deploy was performed.

Before edits, tracked diff and untracked files were backed up privately under
`/tmp/chinhform-before-hostpath-1793899279`. Existing approved runtime-form work
was retained. Domain/source/capability/planner/renderer/round-trip/Preview tests
pass, including all five mount sources on app/init/sidecar for Deployment,
StatefulSet and CronJob. Mutation checks reject changed host path/type/read mode.
Frontend focused suite: 100/100 tests, typecheck/lint/build pass; existing lint,
act and bundle-size warnings remain. No full-repository green claim is made;
the unrelated baseline failures documented above were not changed.

The refreshed dev backend preserved existing volumes/networks and prepared a new
standard source. Only the receipt-owned app Definition/environment was explicitly
upgraded. The retained app was extended through the real form with node-data
source `/var/local/chinhform`, type Directory and read-only mounts in all three
container roles. Save/reopen/resave equality and parsed Preview passed at
2026-11-05T17:28:53.378Z. Preview contains exact hostPath path/type/mode together
with PVC, emptyDir, ConfigMap and approved Secret references; secret bytes remain
absent. The same Tailscale editor URL was independently opened in the in-app
browser with the real backend, and hostPath controls/values/modes were visible.
UI reference: existing WorkloadEditorPage/AppShell/ExecutionFields with shared
app.css and field-grid controls. Review retained existing identity and compact
source help, using translated catalogs.

Native Kubernetes hostPath execution, node placement/permissions, PVC persistence
across a live restart, live Job/sidecar completion and secret rotation still
require separately authorized cluster acceptance. Save/Preview cannot prove these.
Existing tmux sessions and the running Tailscale Console are retained.
