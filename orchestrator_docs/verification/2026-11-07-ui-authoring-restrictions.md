---
id: VERIFY-2026-11-07-UI-AUTHORING-RESTRICTIONS
artifact: verification
status: current
last_reviewed: 2026-11-07
---

# Console authoring restrictions — issue #45

## Outcome and scope

Feature branch `features/git_only_authoring`, based on main `b2d605f9`.
Console hides Workload file import and non-Git source authoring for Terraform
and workload-template Definitions. Backend APIs, stored documents and Catalog
browser remain available. Existing non-Git source provenance is preserved;
changing source explicitly opens Git. Endpoint/existing-cluster forms are unchanged.

Two Console-only switches in `frontend/src/shared/authoringFeatures.ts` are false.
The retained legacy implementations keep regression coverage with switches
explicitly enabled; separate tests exercise the production defaults.

## Checks

- Focused original editor suites: 92 tests passed.
- Production restriction coverage: 4 tests passed, including Git reuse and
  preservation of an existing inline source.
- Additional affected legacy suites: 36 tests passed across two focused runs.
- Typecheck, lint and build passed; lint retains the pre-existing
  `PlatformSettingsPage.tsx` effect cleanup warning. Build warns on chunk size.
- Documentation checker and whitespace checks passed; PlantUML is unavailable,
  so its optional local check is skipped. No diagram was changed.
- Full frontend: 976 passed, 3 failed across 101 files / 979 tests.
  `test/e2e/terraform-v2-labels.test.mjs` and
  `test/e2e/v04-internal-full.test.mjs` each fail identically on unmodified
  main `b2d605f9` (17 other tests in those suites pass on main).
  `src/features/observability/observability.test.tsx` has one timing failure
  in the full run; its isolated rerun passes all 5 tests on both task and main.
  Observability source/tests are unchanged. Full suite is not reported green.
- Go tests were not run because no backend code changed; the required kind
  gate remains unavailable as described below.
- The old App deep-link assertion was made stable and aligned with supported
  workload selection; all 36 App tests pass. No production route behavior changed.

## Real browser and visual review

A separate Compose project `poc-git-only-authoring` uses isolated images and volumes.
Browser signs in as the stack-owned admin through its feature Tailscale hostname.
Terraform and workload-template Definition editors expose Git only. Workload
editor has no import button or file input; filling the form and saving a real
pending draft returns HTTP 200. The final UI also inspects the public Git
repository at exact main commit `b2d605f9ec22b133dc408edae3aae20b0c07401e`,
path `backend/template-sources/standard`, returning HTTP 200 and a verified Git
source pin. No workload was deployed.

Application creation on an unconfigured fresh stack correctly returns
`PLATFORM_CD_REQUIRED`; the Workload editor check uses an explicitly seeded
fixture Application from the existing acceptance seed, not a CD/deployment claim.
The fixture Application and pending draft remain available in the trial stack.

Reference: existing AppShell, shared styles/Button, ResourceDefinitionsPage and
WorkloadEditorPage. Actual browser screenshots reviewed outside Git show the
same shell and layout with the source alternatives and redundant import tab bar
removed. No extra explanatory banner was added; the advanced-document notice
explains view-only behavior without inviting a disabled upload action.

## Remaining gate and retained state

The default kubeconfig has no contexts, so there is no configured access to
the authorized kind cluster from this checkout. Full kind verification has not run; do not
claim that gate passed or merge before acceptance and the required verification.
No shared cluster or existing stack was changed.

The feature Compose stack, its isolated PostgreSQL/Vault data and images remain
running for review. Admin password is in the owner-only file
`~/.config/poc-feature/git-only-authoring-admin-password`; it is not in Git.
Evidence/logs are outside Git under `/tmp/issue45-*` and are temporary.
Restart/cleanup uses the [feature handoff runbook](../operations/tailscale-handoff.md#bản-thử-của-tính-năng)
with feature name `git-only-authoring`. The verified private URL is supplied only
in the user report, not in this public record.
