---
id: VERIFY-20261025-OBSERVABILITY-MAIN-INTEGRATION
artifact: verification-record
status: evidence
last_reviewed: 2026-10-25
related: [ADR-023, UC-09]
---

# PR #19 integration with main

The user authorized merging [PR #19](https://github.com/pr3s3nt/poc/pull/19).
The observability head `e9b985c` conflicted with main `28784f6`, which includes
Account Diagnostics and the CI test corrections from PRs #17/#18.

Resolution preserves both database aggregates, runtime services and HTTP routes,
both Console route flows (including diagnostic deep-link props), both message
catalogs and documentation sections. Diagnostics retains published ADR-022;
observability becomes [ADR-023](../architecture/decisions/ADR-023-native-observability.md)
with references updated. The combined ERD and renamed telemetry class diagram
were rendered again. No existing database or cluster was modified.

## Independent checks on the combined tree

- Disposable PostgreSQL 16, `REQUIRE_POSTGRES=1`: telemetry/diagnostic/setup/role/
  baseline tests pass (13.066s). Full store and bootstrap packages pass (5.023s /
  16.883s); scoped observability/diagnostic HTTP tests pass (0.255s).
- Race tests pass for observability, account diagnostics and Kubernetes adapters
  (1.129s / 1.609s / 11.382s). `go build ./...` passes.
- Frontend typecheck, lint, production build and six focused suites pass:
  **72/72 tests**, covering App routing, route parsing, both feature views and
  message catalogs. Build retains the existing large-chunk advisory.
- Documentation checker with required PlantUML passes using the Ubuntu
  `1.2020.2` package used by the CI installation; whitespace checks pass.
- The named/labelled temporary PostgreSQL container and its anonymous volume
  were removed in a finally block; container absence was independently checked.

The [48/48 real Kubernetes acceptance](2026-10-25-native-observability-live.md)
remains evidence for the original product head; it was not repeated or relabelled
as a live acceptance of this merge tree. This integration check changes no
collection/query/UI behavior. Live EKS verification and retained-database upgrade
remain outside scope. Final GitHub CI and merge status are recorded on PR #19.
