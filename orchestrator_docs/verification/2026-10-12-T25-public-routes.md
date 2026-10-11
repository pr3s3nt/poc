---
id: VERIFY-2026-10-12-T25
artifact: verification-record
status: evidence
last_reviewed: 2026-10-12
related: UC-06, UC-16
---

# T25 — Only publicRoutes, local verification

Task LEG-09 on `refactor_ui`, implemented by Claude in owned tmux session
`codex-T25-20261012`, reviewed by Codex. Initial working tree was clean;
no user-owned changes were staged. Canonical docs were changed before code:
UC-16 BR-12 and UI/realization, UC-06 routing spec/realization, OC-07,
domain objects and schema document. Sequence/VOPC inspected; participants and
Save → Preview → Deploy collaboration unchanged, no PlantUML edits needed.
UC-16 has no separate api-mapping artifact; shared operation contract owns it.

## Outcome and review

`Service.PublicPort` and its route alias are removed. JSON Service decoding
rejects any `publicPort` presence, including empty/null/alongside valid routes,
in Score and Deployment Set projections. It also retains strict decoding at
Service/port/route depth. Save/import fail before draft writes/version advances;
old stored drafts fail explicitly rather than losing their route. HTTP reports
400 `INVALID_SCORE`, trusted field `service.publicPort`, without submitted
values/provider causes. Console explains the removed field in Vietnamese,
blocks old drafts and never converts them. Form serializes only publicRoutes.
Public route iteration remains nil-safe in planning, deployment and transition;
canonical route ordering/default comparisons and protected literal binding
security remain intact. Product ingress script uses current routes syntax.

Review caught the first custom decoder losing nested strictness; Claude fixed
it and added regression coverage before final validation. Browser coverage was
extended from import/image edit to path edit and editor reopen. Reference
fixtures have no publicPort and stayed untouched; no conformance adapter was
needed. `match.go` untouched. Whole repository search leaves alias mentions
only in rejection code/tests, current removal documentation/plans and unchanged
historical evidence. No compatibility rewrite remains.

## Checks

All final checks exit 0. Logs/exit files are outside Git under
`/tmp/poc-T25-handoff-20261012`:

- `cd backend && env -u ORCHESTRATOR_POSTGRES_TEST_URL -u ORCH_KIND_VERIFY go test ./...`
  and `go build ./...`: `go-test-review.log/.exit`, `go-build-review.exit`.
- Conformance included in full suite; separate uncached run passed:
  `conformance-review.log/.exit`. No fixture expectations changed.
- Frontend typecheck/lint/test/build: `fe-review.log/.exit`, 22 files/429 tests.
- `bash -n backend/test/integration/public-ingress-kind-verify.sh backend/test/integration/refactor-ui-local.sh`.
- Codex independent uncached tests of domain/environment, planning/score,
  application/workloadconfig, delivery/http, test/e2e and test/conformance passed.
- `python3 scripts/check_docs.py` and `git diff --check` passed.

Tests cover alias value/empty/null/combined cases, unknown nested fields,
no-write save/import/list failures, HTTP safe code/field/state invariants,
multi-path round-trip and old-draft editor blocking. Existing route conflict,
undeclared port, distinct-path, reference/literal, no-op and retry suites pass.

## Browser evidence

Final run command (headed, Vietnamese captions):

```bash
bash backend/test/integration/refactor-ui-local.sh --scenario T25 --headed --captions vi --evidence /tmp/poc-T25-handoff-20261012/evidence-T25-review
```

Run exit 0, result PASS, seven assertions and nine phase marks. UI steps:
sign in, create disposable Application, import two valid routes, edit image and
`/admin` → `/ops`, save, reopen Edit to confirm `/` → http and `/ops` → admin,
cancel, pending Preview, then import old alias and observe safe rejection with
no save/conversion. Backend GET assertions confirm exact payload/persisted draft.
Final empty/null/combined direct API probes are labelled separately from UI
product steps; draft version/list remain unchanged.

Artifacts: `T25-raw.mp4`, `T25-captioned.mp4`, captions files, `marks.json`,
`assertions.txt`, `result.txt`, `playwright.log` in that evidence directory.
Captioned video 1440×996, 94.6 seconds; Codex full decode exit 0, reviewed
edited-route/reopened/error frames and captions, no credentials or covered UI.
Route reopen correctness is asserted; the video viewport at reopen shows the
editor header. Edited-route frame clearly shows `/` and `/ops`.

T08 regression PASS at `/tmp/poc-T25-handoff-20261012/evidence-T08`, including
single/multiple/invalid/renamed ports, imported overrides/aliases, stale/BUSY.
Codex full decode exit 0, reviewed invalid-route frame and Vietnamese caption
strip. After final decoder/test fixes T08 was not rerun: editor product code
unchanged from that pass and full suites plus final T25 passed.

Earlier T25 evidence is retained: `evidence-T25-run1-marks-short` passed all
assertions but failed video phase-count gate (7 marks, minimum 8); `evidence-T25`
passed after adding a mark. Final reviewed run adds route edit/reopen coverage.
Nothing uploaded; wrapper cleans only run-owned processes/disposable state.

## Limits and handoff

Real HTTP/planner with fake Kubernetes/resource adapters and local disposable
state; T25 uses no page.route response projections. T08 retains labelled Secret
catalog/stale/BUSY projections. No live Ingress or retained resource proof.
No PostgreSQL opt-in, kind/AWS/Vault integration or retained DB/cluster mutation.
The changed kind script was syntax checked only. Old documents with the removed
field require explicit correction/dev reset at T29; no backfill or migration.
Current state/code map/traceability/refactor DONE updated in this task commit.
