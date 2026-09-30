---
id: VER-2026-09-30-REGISTRATION-HARDENING-LOCAL
artifact: verification-evidence
status: evidence
last_reviewed: 2026-09-30
related: UC-02, UC-03, UC-04, I06-10
---

# Registration hardening — local verification

Claude wrote code/tests/recording in tmux; the coordinator owned documentation,
review and publishing. Review required explicit missing/foreign Connection guards,
complete race-winner comparisons, criterion-failure rollback, safe error projection
and truthful UI outcomes when POST succeeds but list reload fails. Forms freeze
during submission. Seed upserts remain separate from insert-only registration.

## Independent checks

Backend `go test -count=1 -race ./...`, `go build ./...`, `go vet ./...` passed
against an owned disposable PostgreSQL 17 server on localhost. PostgreSQL reopen,
repository/service/HTTP concurrent same-key registration and injected second-criterion
failure rollback ran, not skipped. One registration wins; the losing request cannot
replace its fields/criteria. Focused tests also cover tenant/reference validation,
bounded strict HTTP, safe unknown errors and no-restart Preview consumption.

Frontend typecheck/lint, 13 test files/68 tests and build passed. Existing tests
were retained; redundant repository race repetitions reduced from five to one.
Playwright demonstrates UI happy paths; code tests retain concurrency, rollback,
error and deferred-response cases that the recording cannot prove.
Shell/Node syntax, documentation/PlantUML and diff checks passed.
Only owned throwaway databases were used; no kind/AWS environment was changed.

## Recording

[MP4](https://github.com/pr3s3nt/poc/releases/download/acceptance-recordings/uc02-04-human-local-20260930-080921.mp4)
on the existing public acceptance release, containing local test data only.

- Runner: `backend/test/integration/uc02-04-video-local.sh`, exit 0.
- Local run: `/tmp/uc02-04-video-20260930080921/`, 13 real review stages.
- H.264, 1440x900, 15 fps, 190.6 seconds, 1,477,365 bytes.
- SHA-256: `d1f70c87787e505a511aca51f49b03c634b40dbcc1bc9796ea92890794cd3ee9`.

Platform Engineer registers cache input/output schema, sees duplicate and invalid
input rejection, registers supported postgres-fast with class criterion, and
views seeded READY Connection context plus invalid Connection rejection. Developer
then creates an Application and types Score; Preview matches postgres-fast without
restart. Real address bar, visible pointer, mouse clicks and sequential typing
are recorded; no hidden API fixture creates the feature data. The coordinator
reviewed the script and sampled duplicate, Connection and matched-Definition frames,
and decoded the entire MP4 without error, not manually watched every frame.

## Boundaries

The recording does not show successful Connection registration/cluster verification.
Local success tests explicitly use a test verifier. PostgreSQL proof is separate
from the JSON-store UI video. No actual infrastructure provisioning is claimed.
New types validate Score contracts but need runtime-supported Definitions to match.
AWS credential storage/binding, identifier/reserved-key policy and Driver Inputs
shape restrictions remain unresolved. UC-16 parse/draft Save input validation is
a pre-existing gap; planning validates params. IMP-002/I06-10/M02 remain open.
