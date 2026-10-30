---
id: VER-20261030-RECONCILIATION-MERGE
artifact: verification-record
status: current
last_reviewed: 2026-10-30
related: [ADR-027, ADR-029, ADR-030, UC-06, UC-08]
---

# Reconciliation integration into main

User authorized merging `features/reconciliation` into main. Source feature head
is `d3e9383`; main base is `93ebbdf`. Initial working tree was clean. Integration
preserves both features, without deploying or changing a retained installation.

## Integration changes

- Resolve concurrent Console, HTTP/store, CLI and documentation additions while
  preserving current account administration, Git settings and workload catalog.
- Assign reconciliation ADR-030; retain ADR-027 for PostgreSQL upgrades and
  regenerate the affected PlantUML PNGs.
- Register schema version 3, immutable `003-reconciliation.sql` and complete
  frozen `version-003.sql`; released SQL references/migrations stay unchanged.
- Upgrade supported historical sources through existing transitions, then v2→v3
  inside the owner transaction. Preserve installation/domain/state identities and
  Git settings. Existing Environments remain DIRECT/off with zero epoch/revision;
  no historical execution baseline or UID evidence is fabricated.
- Revoke public access on new functions/tables and grant only new table/column
  privileges. Runtime still cannot perform DDL or write upgrade history.
- Extend populated-source preservation/concurrency/repeated-setup and rollback
  tests through version 2. Preserve permission-denied classification when reading
  upgrade history. Future-history rejection fixture now uses version 4.
- Add Console navigation integration coverage for CD, platform settings, Admin
  users and password controls together.

## Validation

Validation results are finalized before the merge commit; local command output
is outside Git under `/tmp/reconciliation-merge-*`.

- Migration immutability/version guard: passed against origin/main.
- Documentation checker and diff whitespace check: passed during integration.
- Go build: passed.
- Focused application/delivery, drift, bootstrap and HTTP checks: passed.
- Real PostgreSQL migration checks: passed, including populated v2→v3, retained
  state/runtime isolation, repeated/concurrent setup and v3 rollback. Test server
  is a new task-owned PostgreSQL 16 container, never a retained database.
- Frontend typecheck, lint and production build: passed. Lint retains one existing
  PlatformSettingsPage hook warning; build retains the large-chunk warning.
- Frontend final suite: all 981 tests / 92 files passed with two workers. Earlier
  merged-catalog errors were fixed; the initial parallel log-follow timing failure
  passed both its focused rerun and the final full suite.
- Backend full invocation completed with real PostgreSQL required (`go test -p 2
  -timeout 30m ./...`). Terraform execution passed (877 seconds), as did the other
  packages except the three already identified failures below. The full invocation
  therefore exited nonzero; it is not reported as a clean full-suite pass.
  - PostgreSQL future-history fixture still used version 3 as an unknown version;
    it now uses version 4. Missing history SELECT privilege was misclassified
    when pgx returned the permission error while advancing rows; classification
    is fixed. Both failed groups and populated v2/v3/rollback tests passed in
    the focused PostgreSQL rerun (39 seconds, REQUIRE_POSTGRES=1).
  - The acceptance backend Redis auth pool test initially reported unreachable
    instead of auth. No acceptance behavior/assertion was weakened. Its focused
    rerun passed, then the entire acceptance backend package passed (5.6 seconds).
  - Following the repository's focused-validation policy, unaffected successful
    groups were reused rather than rerunning the entire backend checkpoint.
- Changed shell scripts passed bash -n; changed JavaScript runners passed node
  --check. PlantUML PNGs were regenerated successfully.
- Task-owned PostgreSQL container and Vite process were removed after checks;
  existing user environments and sessions were preserved. Initial tree had no
  user-owned changes and no unrelated changes are included.

## Browser and execution limits

Headless Chromium opened the actual Console AppShell and Secret Stores reference,
then navigated to CD. These are product screens using intercepted fixture API
responses, not live CD acceptance. The CD screen preserves current sidebar,
fonts, spacing, controls, Git settings and Admin navigation. No JavaScript page
errors were observed. The touched CD empty state contains only its title, list,
actions and empty message; no additional explanatory content is necessary.
Screenshots are outside Git at `/tmp/reconciliation-merge-reference.png` and
`/tmp/reconciliation-merge-cd.png`.

Feature-branch live evidence remains at its original revisions. This integration
does not rerun Kubernetes/Fleet, cloud mutation, controlled drift or repair against
real targets. No staging database upgrade, application deployment or automatic
infrastructure-repair policy is authorized or performed. Infrastructure repair
continues to require explicit plan review and confirmation under ADR-030.
