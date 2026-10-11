---
id: VERIFY-2026-10-11-T17
artifact: verification-record
status: current
last_reviewed: 2026-10-11
---

# T17 — Public existing-cluster registration closure

Branch: `refactor_ui`. Scope: RD-07 backend, UI-02 backend, LEG-03.
Initial working tree was clean; no user-owned changes remain.

## Outcome and review

Canonical ADR-013, UC-03 BR-08/BR-17, realization/sequence/API mapping,
UC-06/08 execution contracts and OC-04 were updated before code delegation.
The catalog service rejects every public `existing-cluster` driver before
catalog/connection lookup or persistence. The reserved builtin key is still
rejected for every driver. Obsolete public static schema/acceptance paths were
removed; supported Terraform/Kubernetes validation remains covered.

Codex reviewed all five changed Go files. Service cases cover arbitrary/reserved/
invalid keys, valid/missing/unknown/AWS connections, missing/other profiles and
missing criteria. HTTP cases prove validation 400 and unchanged catalog contents;
reserved-key rejection is also tested with Kubernetes. The registration e2e
flow now uses its Environment Connection without registering an authored cluster.

Builtin constructor, enum, executor, matching, seeds, reference fixtures and
ReferenceCluster loader were not modified. Existing planning/provisioning tests
retain idempotence, collision, forged-match and FK admission ordering coverage.
No records were deleted and no new compatibility/deletion API was introduced.

## Checks

Claude logs and handoff: `/tmp/poc-T17-handoff-20261011/` (`summary.md`,
`test.log`, `build.log`, `exit.txt`).

- `cd backend && env -u ORCHESTRATOR_POSTGRES_TEST_URL -u ORCH_KIND_VERIFY go test ./...`: exit 0; full suite including conformance passed (unchanged packages used Go cache).
- `cd backend && go build ./...`: exit 0.
- Codex independent `cd backend && env -u ORCHESTRATOR_POSTGRES_TEST_URL -u ORCH_KIND_VERIFY go test -count=1 ./internal/application/catalog ./internal/planning ./internal/application/provisioning ./test/e2e`: exit 0, all four packages passed uncached.
- `plantuml orchestrator_docs/usecase/UC-03/sequence.puml`: exit 0; tracked PNG regenerated.
- `python3 scripts/check_docs.py` and `git diff --check`: exit 0 after final documentation updates; installed PlantUML validates diagrams.

No new Playwright scenario/video: T17 is API/unit-only, and T02 already recorded
form removal. Frontend and shell files are unchanged, so V-FE/shell checks are
not required. SQL opt-in, kind and AWS checks were not run: no disposable SQL or
external mutation was authorized. Local builtin FK ordering tests are simulated,
not real PostgreSQL integration. Existing retained DB/cluster/deployments remain
untouched; their reset is gated separately by T29.
