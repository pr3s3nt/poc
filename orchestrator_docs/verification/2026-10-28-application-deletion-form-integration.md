---
id: VERIFY-20261028-APPLICATION-DELETION-FORM-INTEGRATION
artifact: verification-record
status: evidence
last_reviewed: 2026-10-28
related: [UC-18, UC-16, UC-12, ADR-025]
---

# Application deletion and workload variable form integration

Integrated source: merge `be671f7`, parents `f542766` (Application deletion)
and `54dd42e` (direct variable form, including design `fc23308`), based on
`4764b11`. `features/diagnose` was explicitly excluded; its unique commits
were not merged. The local deletion feature was pushed before integration.

**PASS:** final integrated browser run R4 (15 assertions, no browser errors,
verified clean teardown), full Go suite/build with real PostgreSQL, frontend
checks/build and documentation validation.

## Validation completed

- Frontend typecheck, lint and production build passed.
- Frontend full suite: 78 files, **923 tests passed**, no failures/skips,
  `--maxWorkers=2`. Existing React act/bundle-size warnings remain.
- E2E shared helper tests: **6/6 passed**, no skips.
- Integrated backend full suite and build passed with real PostgreSQL:
  `go test -p 2 -timeout 30m ./...` under `REQUIRE_POSTGRES=1`, then
  `go build -p 2 ./...`.
- Documentation checker, diff whitespace check and PlantUML `-checkonly` for
  UC-18 sequence/VOPC and the changed UC-16 sequence passed.
- Regenerated diagram PNGs; UC-16's PNG now matches its changed source.
  Both branches' appended code-map/traceability sections and the complete
  verification index were preserved and checked against both parent documents.
  VI/EN catalogs preserve both branches' changes and contain no duplicate keys.
- The old PostgreSQL identity/reopen test expected an unset lifecycle status;
  UC-18 correctly persists `ACTIVE`. Updated the expected persisted value;
  its focused real-PostgreSQL regression passed.

Initial full Go run found that expectation mismatch and environmental tmpfs
exhaustion while copying Terraform. Its 10-minute package deadline also expired.
The integrated rerun retains all assertions, uses real isolated PostgreSQL,
`REQUIRE_POSTGRES=1`, two package workers, disk-backed `/var/tmp` and a 30-minute
package deadline. **Integrated rerun and build both passed.** Its dedicated
PostgreSQL container/volume, private credential file and Go scratch directory
were removed and verified absent after completion.

## Browser runs

Procedure: [Application deletion form scenario](../../e2e/scenarios/application-deletion-form/README.md).
Real new kind cluster, workload Vault/VSO, Compose PostgreSQL/Vault/backend/
frontend production images, pinned Terraform modules and acceptance workloads.
No fake adapter, intercepted reply, Score file/editor/import or direct
application/workload mutation API.

- R1 `mergeui1028r1` stopped before business actions because legacy frontend
  helpers and `e2e/` loaded two Playwright runtimes. The new scenario resolves
  both to its one locked ESM runtime; the import smoke check passed.
- R2 `mergeui1028r2` stopped at helper construction because the new wrapper
  omitted its Kubernetes read configuration. Supplied the exact owned context/
  kubeconfig and tool handles. No application/workload was created in R1/R2.
- Both failed runs proved Compose containers/volumes, workload Vault, kind
  cluster and run-tagged images absent before the next attempt. Their original
  failure results and private evidence remain.
- R3 `mergeui1028r3`: original runner **PASS, 13 assertions**, clean teardown.
  App `31884b1e-13e4-4a4b-a289-5f187e71f5eb`, deletion
  `650602ba-3425-4bf3-8433-063a4b43cd89` completed in about 91 seconds. Both
  Environments succeeded; staging's three instances were destroyed; exact
  Secret/bundle refs, namespace and PVs were absent, cluster kept serving, and
  name/subdomain reuse plus empty replacement deletion passed.
- Visual review of R3 found the cache row showing FAIL immediately after a
  job reached DONE, despite the earlier pre-job five-PASS check. R3 therefore
  does not prove final cache health. The worker commits DONE before recording
  the Redis write outcome; the runner must verify both. Added post-job and
  post-update UI five-PASS checks and read-only evidence that recorded worker
  writes are nonzero with no failed/unverified writes or unresolved failures.
  No product health or failure guard was weakened.
- R4 `mergeui1028r4`: **PASS, 15 assertions**, with those added checks.

## Final browser outcome

Application `c2f5719f-9bd6-4100-8956-bd3df7498892`, namespace
`app-c2f5719f-9bd6-4100-8956-bd3df7498892-staging`. Form creation began at
2026-10-27 18:36:47 UTC. Three-workload deploy and a one-workload update
succeeded through explicit UI Preview/Deploy.

| Observation | Result |
|---|---|
| Ordinary values, inline Variable/Secret, resource outputs and Service reference | Entered through form controls; Secret input masked and value absent from page text |
| PostgreSQL, Redis and PVC reuse | Both StatefulSet and both PVC UIDs unchanged across the valid update; object count preserved |
| Initial and post-update job | Both DONE in UI; original job/result retained; updated ordinary value present on backend Deployment |
| Final application UI | All five checks PASS after each job; screenshot reviewed |
| Redis evidence | Recorded worker writes 1 then 2; failed/unverified 0; no unresolved failures or overflow; actual HIT counts 2 then 7 |
| Application deletion | Developer confirmation in Settings; editing locked; both Environments SUCCEEDED |
| Owned infrastructure | PostgreSQL, Redis and namespace instances DESTROYED; successful native namespace/PV absence reads; platform cluster identity preserved |
| Vault | Exact Secret and live deployment bundle paths read as 200 before deletion and 404 afterwards; unknown/auth/network errors not treated as absence |
| Audit and reuse | Tombstone and history retained; deleted app GET 404; same name/subdomain accepted with fresh ID, then empty replacement deleted through UI |

Deletion operation `00dfd35d-cc93-4140-b573-dc9687fe8a4c` ran
18:39:23–18:40:52 UTC (about 89 seconds). One Application Secret was purged.
Replacement ID `7b14cf23-7529-407f-932f-a9d4b29a17b6` also reads as absent.

The wrapper reported `scenario_exit=0`, `final_status=0`: exact Compose
containers/volumes, workload Vault, kind cluster and run image tags absent.
Public-evidence secret scan passed. Other retained clusters/installations and
the user's tmux sessions were preserved.

## UI review and limits

Reviewed actual workload-form, deletion-confirmation and application-result
screenshots. They retain the existing AppShell, source table, shared form/
Dialog/button styles and clear typed-name/destructive-scope confirmation.
No new product copy, shell, navigation or identity was introduced. Secret values
remain masked/referenced. The workload form and confirmation match their
integrated references; existing background Settings helper copy was outside
this task's UI change scope.

This is real kind acceptance. Existing UC-18 limits (platform output-secret and
delivery policy/role retention, PostgreSQL-only lifecycle, in-process execution)
remain explicitly tracked by [IMP-038](../implementation/deviations.md#imp-038--application-deletion-2026-10-27).

Private evidence root: `/home/thanhnt1/.codex-runs/merge-ui-20261028/`.
No credential, state, browser session or raw runtime artifact is committed.

## Earlier AWS run

`xdaws1027r1` failed before deletion on unfulfillable Spot capacity. Original-
state operator cleanup, six native AWS absence layers and local teardown are
verified separately in [the recovery record](2026-10-28-application-deletion-aws-cleanup.md).
That failure is not converted into an AWS product-deletion PASS. No new AWS
deployment is part of this integrated kind run.
