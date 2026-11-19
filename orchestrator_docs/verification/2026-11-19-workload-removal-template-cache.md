---
id: VERIFY-20261119-WORKLOAD-REMOVAL-TEMPLATE-CACHE
artifact: verification-record
status: current
last_reviewed: 2026-11-19
---

# Workload removal with an unavailable template, issue #80

Staging reported `WORKLOAD_REMOVAL_FAILED` and a backend log with
`code=TEMPLATE_UNAVAILABLE type=*fmt.wrapError`. The report established the
failure category. The staging cache and renderer pin were outside this run.

The regression test deployed two workloads using the real standard template
renderer and fake infrastructure. It then moved the only matching bundle out
of the test-owned cache. Before the fix, the first REMOVE failed with
`TEMPLATE_UNAVAILABLE`, because its intermediate candidate Set still included
the second workload. No workload was removed.

Commit `2321fb2e` limits renderer runtime preflight to steps that apply a
workload. REMOVE continues to use the stored target, inventory, ownership and
delivery evidence required by [UC-07](../usecase/UC-07/specification.md#git-bundle-updates-and-removal--adr-021).
After the fix both workloads were removed. The same test verified update still
rejects an unavailable template. The related service-reference checks passed.

Application deletion API tests passed with required real PostgreSQL and fake
cleanup adapters. Go build and documentation validation passed. The focused
Deployment run failed the same eight tests as the preceding backend without
the correction.

The rebuilt backend image was deployed on the retained kind installation using
the same frontend source already running there. An isolated pre-upgrade database
restore matched the retained identities, the owner Job passed, and both
Deployments were Ready. The authenticated Tailscale Console loaded the real
Application and catalog APIs. Earlier deleted test Applications stayed absent.
The staging installation remains an operator rollout.

The full kind run passed 1,621 Go tests and the Go build. Compared with
`04feb705`, the exact sets of 103 Go failures, eight skips and 20 failed packages
matched. The new regression passed in the full run. Frontend passed 1,111 tests
across 117 files, typecheck, lint and build. The disposable kind test namespace
was verified absent.
