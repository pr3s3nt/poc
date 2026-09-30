---
id: VER-2026-09-30-UC07-UPDATE-REMOVE-LOCAL
artifact: verification-evidence
status: evidence
last_reviewed: 2026-09-30
related: UC-07, I06-09
---

# Update/remove — local verification, 2026-09-30

Claude wrote code/tests and the UI recording in tmux. The coordinator owned
canonical documentation, independently reviewed changes and required fixes for
logical resource ownership, escaped Service placeholders, strict request bodies,
failure-text disclosure, stale responses and misleading post-deploy reload
errors. No live Kubernetes/AWS environment was changed.

## Independent checks

- Backend `go test -count=1 -race ./...`, `go build ./...` and `go vet ./...`
  passed with the test URL targeting an owned disposable PostgreSQL 17 Alpine
  server bound only to localhost.
- `TestPostgresRemove_CommitsSetStatusAndMarkerAtomically` ran explicitly with
  `-v -race`: passed, not skipped. An injected marker-write failure after the
  current-set compare rolled back both current pointer/version and marker;
  successful removal persisted current set, SUCCEEDED and UNREFERENCED through
  store reopen. Identity, outputs, executor state and fingerprint were retained.
- Tests cover shared-resource consumers/last reference/re-reference, Organization
  and Environment scope, excluded Application scope, runtime/final-version
  failures, other modules preserved, scoped Delta, stale before/shared conflict
  without external side effects, and final Service-reference validity.
- Strict scoped draft/pending API tests cover omitted/null/negative versions,
  unknown fields, trailing/non-object JSON, oversized trailing bytes (413),
  session/tenant scope, stale version/token (409), safe runtime/store messages,
  and masking legacy unsafe FailureReason in history reads without rewriting it.
- Frontend typecheck/lint, 12 test files/55 tests and build passed. Tests include
  confirmation/Undo, busy controls through post-deploy reload, preserved outcome
  when reload fails, truthful conflict/retry, obsolete responses and editor input
  preservation with explicit stale-state reload and no automatic resend.
- Production dependency audit reported zero vulnerabilities. Shell/Node syntax,
  documentation/PlantUML checker and `git diff --check` passed.

The coordinator's owned container was stopped/removed after testing, deleting
only its throwaway database state. Claude also removed its separate owned test
container. Some existing adapter tests mutate the test URL's base database, so
these runs used disposable servers rather than an existing database. Existing
containers, cluster workloads and cloud resources were not removed.

## Review recording

- [MP4](https://github.com/pr3s3nt/poc/releases/download/acceptance-recordings/uc07-human-local-20260930-073724-10075.mp4)
  on the existing public release, with only local fake/test data.
- Command: `bash backend/test/integration/uc07-video-local.sh`.
- Final run `uc07-video-20260930073724-10075`, exit 0.
- H.264, 1440x900, 15 fps, 205.13 seconds, 1,611,605 bytes.
- SHA-256: `7152e8e4f7039d8b4286c3d47cfb27a95b6909ce54eb5da446e26a8f6d0d3e82`.
- Local evidence `/tmp/uc07-video-20260930073724/`: video, logs, marks,
  metadata and sampled frames; not tracked source.

| Approximate time | UI behavior |
|---|---|
| 0:01–1:40 | Sign in, create Application, enter api with PostgreSQL dependency and web, Preview/Deploy both. |
| 1:50–2:14 | Edit web image and CPU request, preview only its update, deploy; api remains Ready. |
| 2:20–2:38 | Native Delete confirmation: cancel, confirm pending deletion, then Undo web. |
| 2:45–2:59 | Confirm api deletion, preview unused-resource warning (not destroyed), deploy; web remains Ready. |
| 3:04–3:21 | Open remove Deployment detail, inspect four history entries, sign out. |

All Application/workload setup and feature actions use UI input on camera;
only test accounts/catalog are seeded. The headed Chromium address bar is real
and stays visible. Mouse movement/clicks and sequential typing are input events.
Native dropdown/confirmation use X11 input, not hidden dialog acceptance. Views
pause for reading. The coordinator reviewed the script and sampled update,
confirmation, removal-preview and history frames and independently decoded the
entire MP4 without error; this is not a claim of manually watching every frame.
The runner stopped its owned processes and removed temporary JSON state.

## Boundaries

Fake executors make the video a UI/orchestration demonstration, not proof of
live resource provisioning or Kubernetes removal. PostgreSQL atomicity is
separately tested, not shown in the JSON-store video. Service-consumer batch
validation and failure/conflict paths are covered by tests, not this recording.

Environment deployments mark only their READY Environment/shared/workload
resources. Application-scoped cross-Environment cleanup, non-READY cleanup,
automatic destroy, rollback and concurrent deployment recovery remain outside
scope. Pending batches still commit sequentially and can be partial. Public-route
failure retains the existing 502 response shape without the partial report body.
Safe failure summaries intentionally omit raw runtime diagnostics; legacy
direct deployment API/error handling remains outside this scoped UI/API slice.
