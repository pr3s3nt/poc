---
id: VERIFY-DIAGNOSTICS-CI-20261025
artifact: verification-evidence
status: evidence
last_reviewed: 2026-10-25
---

# Post-merge CI test stabilization

After the [37/37 real merged-main browser acceptance](2026-10-25-diagnostics-main.md),
main CI on `7f3886b` failed the PostgreSQL fencing fixture (`sweep: 0`). An earlier
push CI on `21d18fb` failed the template deadline fixture (`TEMPLATE_OUTPUT`).
Although a subsequent unchanged-main run passed, these intermittent failures were
investigated rather than accepted as resolved by rerunning.

[PR #18](https://github.com/pr3s3nt/poc/pull/18), code checkpoint
`df2fe22c591ec41d8036866890f079eecb009686`, changes only two test files:

- `envops/fencing_test.go`: `Manager.Begin` implicitly sweeps. The fixture's50ms
  stale threshold could expire the original claim during the initial Busy check,
  before its explicit sweep. Test managers now have hour-long heartbeat/staleness
  settings, with future-cutoff sweeps driving interruption explicitly. All exact
  interruption counts, stale-owner write/release guards and recovery-fence
  assertions remain. JSON and real-PostgreSQL regressions delay Busy admission by
  300ms, beyond the old threshold.
- `renderengine/renderengine_test.go`: the nested loop previously emitted output
  and could hit the output cap before500ms. Silent loops now exercise CPU timeout,
  real child kill/reap and the unchanged prompt-deadline assertion.

No runtime code, lease threshold or template resource limit changed. Comparing
`31ef867..df2fe22` lists only those two `_test.go` files, so the prior real
runtime acceptance remains applicable; no infrastructure rerun is needed for
this test-only correction.

Independent checks used the committed checkpoint and a fresh, explicitly named
and labelled PostgreSQL container/volume (`diagnose-ci-review-20261025`):

- Before correction, the renderer deadline test failed repeatedly with
  `TEMPLATE_OUTPUT` under `-count=10`; the original fencing cases passed locally
  ten times, confirming the issue depended on admission timing seen in CI.
- Both complete packages passed with `REQUIRE_POSTGRES=1`:
  `go test -p 2 ./internal/application/envops ./internal/renderengine -count=1`.

- Independent stress/race verification passed with real PostgreSQL:
  `go test -race -p 2 ./internal/application/envops ./internal/renderengine -run
  'TestStaleOwner|TestWorkerIsKilledAtDeadline' -count=10`.

Source-bound logs are outside Git under
`/home/thanhnt1/projects/diagnose/evidence/diagnose-coordinator-20261025/review/ci-fix-*`.
The pre/post-merge runtime evidence and its capability limits remain unchanged.
