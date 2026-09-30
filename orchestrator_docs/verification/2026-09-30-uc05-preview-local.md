---
id: VER-2026-09-30-UC05-PREVIEW-LOCAL
artifact: verification-evidence
status: evidence
last_reviewed: 2026-09-30
related: UC-05, I06-08
---

# Standalone Score Preview — local verification, 2026-09-30

Claude implemented code/tests and the recording in tmux; the coordinator owned
documentation, independently reviewed the changes and requested fixes for
public error disclosure, snapshot write guards, YAML aliases and obsolete UI
responses. No cluster/cloud deployment occurred.

## Independent checks

- Backend: `go test -count=1 -race ./...`, `go build ./...`, `go vet ./...`
  passed with `ORCHESTRATOR_POSTGRES_TEST_URL` targeting an owned, disposable
  PostgreSQL 17 Alpine container bound only to localhost.
- `TestPostgresPreview_RepeatableReadAndNoWrites` also ran explicitly with
  `-v`: passed, not skipped. Preview did not write for valid/invalid requests;
  a concurrent commit after the first snapshot read remained invisible.
- Preview/deploy parity tests compare plan hash, base state and persisted Delta
  for add/update/remove. Tests cover empty base, CPU preservation, internal/AWS
  implicit graph, provider-first order, tenant scope, strict request/413,
  no draft/Deployment/token, and fail-closed public errors/projections.
- Frontend: typecheck, lint, 12 files/45 tests and build passed, including YAML
  alias/self-reference errors and late-response rejection after input/scope
  changes. The new local parser uses `yaml`; production dependency audit
  reported zero vulnerabilities in this run.
- Shell/Node syntax, documentation checker and `git diff --check` passed.

The owned test container was removed after verification, deleting only its
throwaway database state. No existing database/container was removed. Some
pre-existing adapter tests write the test URL's base database, which is why a
disposable server, never a staging/production URL, was used.

## Review recording

- [MP4](https://github.com/pr3s3nt/poc/releases/download/acceptance-recordings/uc05-human-local-20260930-065714-19834.mp4)
  on the repository's public release; only fake data and local test accounts.
- Command: `bash backend/test/integration/uc05-video-local.sh`.
- Run: `uc05-video-20260930065714-19834`, exit 0.
- H.264, 1440x900, 15 fps, 214.13 seconds, 2,025,364 bytes.
- SHA-256: `100a23d0cb5327669d957fb92e0a3bcac9140e42d7adb402e98479c4a61fba50`.
- Local evidence: `/tmp/uc05-video-20260930065714/` with MP4, logs, marks,
  metadata and 20 sampled frames. Not tracked source.

| Approximate time | Recorded UI behavior |
|---|---|
| 0:01–1:05 | Sign in; create Application; enter/save/deploy web workload through UI. |
| 1:09–1:47 | Open Preview Score; submit empty input; type YAML and mismatched Workload ID; see server validation. |
| 2:00–2:34 | Correct ID; inspect successful Delta, resource changes, batches, matches, graph and Candidate Set. |
| 2:39 | Switch to production; old result clears. |
| 3:14 | Choose update via native keyboard and type before/after Score; no-change result. |
| 3:25–3:30 | Return to Application: only web Ready, no saved preview draft; sign out. |

The real Chromium tab/address bar stays visible. Pointer movement, clicks and
sequential typing are real input events; native X11 keys operate the Action
dropdown. Important views pause for reading. There are no API fixtures or
hidden substitutes for the recorded setup/feature actions. Only seed accounts
and catalog exist initially. The coordinator reviewed the script and sampled
frames (validation, success, graph, sanitized Candidate, no-change and setup),
and independently decoded the whole MP4 without error; this is not a claim of
manually watching every frame. Owned browser/backend/recorder/Xvfb stopped and
temporary JSON state was removed; reviewer artifacts remain.

## Boundaries and remaining limitations

Fake executors make this a UI/planning verification, not proof of Kubernetes
or AWS runtime changes. AWS implicit planning needs no live AWS credential.
Standalone Preview and direct Deploy share a consistent loader; the UC-16
pending Preview remains its existing separate implementation.

Public views omit graph parameter values, Terraform contracts and raw
Connection/ActiveResource data; literal container variables of other workloads
are redacted. Candidate resource params, images, command/args and caller-owned
Score content are not universally redacted. Do not encode secrets as literals
in Score; only opaque references/placeholders are supported for secrets. No
secret provider is read during Preview. Internal Delta/Candidate/hash are
unchanged by the public projection.

The pre-existing direct `POST /api/v1/deployments` boundary remains outside
this slice; the new Preview endpoint requires the session and full tenant
scope. Catalog read failure now fails before creating a Deployment record,
because snapshot loading is a precondition; pure planner failures retain the
existing PLANNING → FAILED history behavior.
