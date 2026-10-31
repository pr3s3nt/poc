---
id: VERIFY-2026-10-30-CATALOG-RELEASES
artifact: verification-record
status: evidence
last_reviewed: 2026-10-31
---

# Issue #28 — local implementation verification

Branch `features/nangcap`, based on latest fetched `origin/main` at `7b510fa`.
Initial working tree was clean; no user-owned changes were modified. Source and
release design: [ADR-031](../architecture/decisions/ADR-031-versioned-catalog.md),
[specification](../usecase/UC-03/catalog-releases.md),
[implementation/limits](../implementation/catalog-releases.md).

## Local evidence

Real HTTPS Git fixture tests cover annotated tags, stable/prerelease numeric
ordering, tag ancestry outside the tracked branch, credential/CA resolver reuse,
private snapshot cleanup, symlink rejection and complete Definition inspection
without global writes. Service tests cover failed inspection, immutable moved
observations, remote failure retention, atomic publication under source CAS,
Environment isolation, release-only resolution and partial/fenced activation.

Disposable local PostgreSQL 17 is task-owned, with password authentication and
matching backup/restore client tools inside its container. Focused tests cover
populated version-3 upgrade, frozen active-schema equivalence, rollback, preserved
state identities, repeated/concurrent setup, catalog restart/CAS, immutable
release privileges and resource acknowledgement without a global Definition.
No retained installation, staging PostgreSQL, Kubernetes or cloud was mutated.

Frontend full suite: 93 files / 984 tests pass with `npm test -- --maxWorkers=2`.
A preexisting observability timing test failed under the initial heavily parallel
run; the unchanged test passes in the complete two-worker suite. Typecheck,
lint and production build pass. Lint retains one warning in the unchanged Git
settings effect; build retains a bundle-size warning. Related tests also pass
after the final Catalog Preview/selection refinements.

Browser fixture checks use the real product Vite/React UI with mocked API data:
Platform settings reference, Catalog source Save → manual refresh → release list,
and Environment desired selection/differences with applied version retained.
Reference: AppShell, PlatformSettingsPage, SettingsPage/DeliverySelectionPanel,
shared Button, create-form/content-panel/styles. Visual comparison retains shell,
typography, spacing and controls. Advanced fields/help carry nonessential details;
no extra explanatory introduction was added. Screenshots remain outside Git.
These checks are product UI with fake data, not live deployment evidence.

## Final checks

Final local checks completed on 2026-10-31 (Asia/Bangkok):

- `REQUIRE_POSTGRES=1 ORCHESTRATOR_POSTGRES_TEST_CONTAINER=<owned-container> ORCHESTRATOR_POSTGRES_TEST_URL=<private-fixture-url> go test -p 4 -timeout 30m ./...`: PASS. PostgreSQL suite 483.400s; tfexec suite 875.228s; local Go E2E suite 224.293s. No failure/timeout in the final run.
- `cd backend && go build ./...`: PASS.
- Frontend typecheck/lint/build: PASS; full two-worker suite 984/984 PASS,
  final focused UI suite 27/27 PASS after usage/status/style refinements.
- Documentation checker, migration immutability guard against `origin/main`,
  `git diff --check`, regenerated PlantUML and E2E runner syntax: PASS.

Earlier validation identified and fixed memory transaction nesting, unsupported
fresh-baseline upgrade DDL, catalog-only Active Resource provenance and legacy
configuration test fixture coverage. The fresh baseline now declares final column
shape directly; only migration 004 contains upgrade transitions.
Migration guard compares frozen historical SQL against `origin/main`; it does not
modify a released migration or marker. PlantUML sources/PNGs are regenerated.
E2E runner syntax is checked; [live scenario](../../e2e/scenarios/catalog-upgrade/README.md)
is not executed because external mutation/acceptance was not authorized.

Live acceptance still needs application data/Jobs/PostgreSQL/Redis/PVC physical
identity checks, restart/cache recovery and partial provider outcome scenarios in
an explicitly authorized isolated environment. No issue closure or live
acceptance claim is made by this local record.


## Cleanup and handoff

Both task-created PostgreSQL containers were removed after verifying their exact
container IDs and `orchestrator.task=issue28` labels. No task-labeled container
remains. The task Vite server on port 5280 was stopped. Browser fixtures and
private test logs/screenshots remain outside Git; no user tmux session was closed.
The initial working tree was clean and no unrelated user-owned change remains.
