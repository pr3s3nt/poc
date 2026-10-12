---
id: VERIFY-T28-FRESH-BASELINE-20261012
artifact: verification-record
status: evidence
last_reviewed: 2026-10-12
---

# T28 — Fresh persistence baseline

Task: T28 / LEG-08 on `refactor_ui`. Initial working tree was clean.
Codex updated schema/ERD, ADR-011/012 supersession, UC-09 history realization
and reset/backup runbooks before Claude implemented code/tests through the owned
tmux session `codex-T28-20261012`. No pre-existing user work was changed.

## Implemented preparation and review

PostgreSQL uses a complete fresh `baseline.sql` and singleton `schema_baseline`
identity, without the old migration chain. Initialization is atomic under an
advisory transaction lock. Nonempty schemas must have the current marker and
structural catalog; ledger deletion is insufficient. Reopen compares relations,
column type/nullability/defaults, FK/unique/CHECK constraints, partial/expression
indexes, Snapshot triggers and function bodies. Comparison builds a private
reference schema in the same transaction and rolls it back; no existing schema
is repaired. The backend database role needs CREATE privileges for this check.
This is not a zero-DDL implementation, but does not convert product state.

Fresh schema preserves UUID identity, Application metadata-only persistence,
explicit kind-compatible Connection auth, ENVIRONMENT-only scope, ordinary Store,
metadata-only Variable/store-ref-only Secret constraints and existing transaction,
generation, history and operation fencing invariants. Connection defaults are
removed; seed and caller fixtures supply explicit metadata. AWS process seed
semantics remain; no new AWS onboarding is claimed.

JSON requires every current aggregate map, explicit identity/binding/auth/config
metadata and required owner/pointer integrity. It never initializes loaded defaults,
infers Connection metadata or reconstructs workload history. Rejected files remain
unchanged and errors omit values. Current empty/seeded/generation/history reopen,
missing/null aggregates and invalid/retired format rejection have local coverage.

Current PostgreSQL binding/registration/history/operations/multi-backend suites
remain. Historical migration/backfill tests are replaced by fresh/reopen/drift/
reset-rejection tests. Review also
corrected timeout invocation of shell functions, inherited Go flags and cleanup
when Docker creates a resource before its ID is captured.

## Local checks

Logs are outside Git at `/tmp/poc-T28-handoff-20261012/`.

- Independent uncached `env -u ORCHESTRATOR_POSTGRES_TEST_URL -u ORCH_KIND_VERIFY
  go test -count=1 ./...`: exit 0, `codex-go-test-initial.log`/`.exit` and
  `codex-go-test-review.log`/`.exit` after JSON review; stable final run also
  passes in `codex-go-test-final.log`/`.exit`. Conformance and HTTP/e2e pass.
- Independent `go build ./...`: exit 0, `codex-go-build-initial.log`/`.exit` and stable final build
  `codex-go-build-final.log`/`.exit`.
- Independent targeted `go test -race -count=1` for JSON store, planner and ports:
  exit 0, `codex-race-review.log`/`.exit`.
- Claude full uncached Go suite/build/vet and targeted race reported exit 0 in
  `status.txt`, `go-test-uncached.log`, `go-build.log`, `vet.log`, `go-test-race.log`.
- `bash -n backend/test/integration/postgres-baseline-local.sh`: exit 0.
  Independent `--dry-run`: exit 0, `codex-runner-dry-run.txt`; no Docker call or
  resource creation. Runner uses a new evidence path and label-owned tmpfs
  PostgreSQL/container/network, localhost random port, restricted Go environment,
  per-step results and cleanup, with synthetic dump/restore only.
- Documentation checker, regenerated ERD and whitespace pass. Old runtime
  migration/default/backfill helper searches have no remaining implementation.
  Matching core and reference fixtures are unchanged.

## Authorized PostgreSQL execution and independent review

On 2026-10-12 the user explicitly authorized disposable PostgreSQL execution,
corrections/reruns and cleanup, excluding retained IDP/Vault/K8S-4F. Claude resumed
in the task-owned session `codex-T28-resume-20261012`.

| Run | Evidence / outcome |
|---|---|
| `poc-t28-pg-20261012T024110Z-e281a1b7` | `/tmp/poc-T28-postgres-20261012-run1`: FAIL; full/race suites exposed three fixture errors. Build and synthetic restore passed. All logs and statuses retained; cleanup passed. |
| `poc-t28-pg-20261012T024321Z-064bde4a` | `/tmp/poc-T28-postgres-20261012-run2`: PASS; full uncached Go suite, build, PostgreSQL opt-in proof, targeted persistence races and synthetic restore all exit 0. Cleanup passed. |

First-run corrections are limited to tests: the multi-backend bind fixture now
supplies explicit current scope/profile/runtime metadata; concurrent registration
supplies explicit Connection names; workload history writes use
`UpsertWorkloadProgress` while records are nonterminal, which persists both current
and per-Deployment rows. `UpsertWorkloadInstance` alone deliberately does not create
history. Assertions still require one claim/registration winner, persisted history
on reopen and no history fabrication after deletion. No product SQL/catalog code
was weakened or changed after these failures.

Codex independently reviewed `steps.tsv`, every step status, `result.txt`,
`postgres-opt-in-proof.txt`, full/race test logs, synthetic restore and
`cleanup.txt`. Run2 has zero skipped tests. The fresh/idempotent and 8-backend
initialization, failed-initialization rollback, old layout/removed ledger/fake
marker rejection, schema drift cases, SQL constraints, binding/CAS, multi-backend
ownership, registration races, read snapshots and history reopen suites pass.
Private reference schemas leave no residue after successful or rejected boot.
Conformance and HTTP/e2e also pass in the full PostgreSQL-enabled run.
After the fixture corrections, independent non-opt-in full tests/build pass again
in `codex-resume-go-test.log`/`.exit` and `codex-resume-go-build.log`/`.exit`.
Final documentation/whitespace and runner syntax checks pass.

Runner isolation proof in `docker-isolation.json`: no volume/bind mounts, PGDATA
on tmpfs, only the new label-owned network, random port published on 127.0.0.1.
Run2 used PostgreSQL 16 image digest
`sha256:3c5c8892d184f738f4fe282d14ddaa613a38f00f4189d2d94725ebe6f2909ddb`,
Docker 29.5.2 and Go 1.27.1. Per-run test password is redacted in evidence.
The synthetic 500-row dump/restore checksum matches
`c00bb75fe425443e9a0730a8e3d95cc6`; it proves tooling only, not a retained backup.

Both runs report zero remaining containers, networks and volumes carrying their
run label. Independent read-only Docker checks find no T28-owned container/network;
retained Compose services and K8S-4F/Vault containers remain running. No retained
DB, Vault credential/data or cluster workload was read/reset/mutated.

## Limits and handoff

T28 has no browser scenario/video because it is schema/CLI-only. No frontend
files changed; frontend gates and live UI scenarios were not rerun. Kind/AWS/Vault
live mutation and retained backup/reset/rebuild remain outside this task. Current
builds require the separately approved T29 dev reset before use against retained
state; its prepared runbook does not prove a real retained backup or restore.
Fixed test account detection remains a production safety check; protected literal
policy, native rendering omission, generation-0 identities and Agent delivery stay.

All changes belong to T28; no pre-existing user-owned changes were present.
One task commit with prefix `T28:` includes implementation, canonical docs,
traceability, evidence and the DONE row. Find its actual SHA via
`git log --grep '^T28:'`; push destination is `origin/refactor_ui`. The task-owned
resume tmux session is closed at final handoff; unrelated sessions remain.
