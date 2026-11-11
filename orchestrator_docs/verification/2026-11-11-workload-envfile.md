---
id: VERIFY-2026-11-11-WORKLOAD-ENVFILE
artifact: verification
status: evidence
last_reviewed: 2026-11-11
related: [UC-12, UC-16]
---

# Synchronized UI/.env and bulk Secrets — issue #72

## Outcome and scope

Implementation on `features/ui_envfile`, awaiting user acceptance, not merged.
Design `33124178`, implementation `16464b4b`, container isolation/acceptance
`ac6b6614`, atomic metadata reconciliation `47b5dd31`. [Specification](../usecase/UC-16/envfile-specification.md) owns behavior;
[runner](../../e2e/scenarios/workload-envfile/README.md) owns repeatable procedure.
No backend API, database, renderer or deployment contract change.

UI/.env share ordered per-container bindings. Invalid text stays in memory and
blocks Save/stale-table edits, including when the group is collapsed. Text that
becomes valid after metadata changes is published atomically before Save unlocks.
Stable container identities prevent cross-container text transfer after removal. Parser
handles quotes, whitespace, Unicode, multiline, =, empty values, @@, all five
sources and positional duplicate/reference errors. File import reviews collisions
with Keep as default, preserves absent keys, and exports only saved Secret refs.
Markers and bulk selection defer create-only Secret writes to one confirmation;
successes replace their rows, failures/unknown outcomes stay explicit and never
automatically replay. No draft is stored in browser storage/logs/telemetry.

## Automated checks

- 57 focused tests pass across parser, editor file merge, table/bulk lifecycle,
  alias guards and workload-import restrictions.
- `go build ./...` passes. Full Go tests were run with real disposable PostgreSQL
  (`REQUIRE_POSTGRES=1`) and remain red on existing main fixtures; they are NOT
  reported as passing. Baseline comparison uses a complete pristine `origin/main`
  archive at task base `dfa6470c` in the same kind namespace. Remote main
  advanced to `b8fb28ee` during the task; its backend and referenced frontend/catalog
  fixtures were rechecked and remain identical. Task and main backend Git trees are exactly
  `e284ce524f63baf50d97bed36a347d15c8555a45`, and no Go test reads the changed
  frontend product files.
- Representative main failures reproduced: `TestBaselineEnforcesCurrentConstraints`
  still inserts retired `applications.subdomain`; `TestInspectRejectsUnsafeSourcesWithoutFetching`;
  catalog capability, configuration rename, deployment removal, pending renderer,
  preview/provisioning, real-profile, UC-12/16 HTTP, v2cleanup and historical V05
  assertion-list tests. These are outside this frontend change. Full baseline
  details: `go test -p 2 -parallel 4 -timeout 20m ./...` completed with 13 failing
  packages on pristine main, without a timeout; see the table below.
- Full frontend on final product source passes in kind: 107 test files / 1025
  tests, typecheck, lint (one existing PlatformSettingsPage warning) and build.
- Documentation checker, `git diff --check`, and Node syntax check pass.
  PlantUML is unavailable locally; no diagram source was changed.

Early test-container attempts omitted Go tests through production .dockerignore,
lacked sibling repository fixtures/Bash, or exhausted the default PostgreSQL
connection limit. Those attempts are excluded from passing-test evidence; the
initial no-test Go claim was corrected explicitly in the issue. The final runner has
the full repository structure and Bash; the final PostgreSQL baseline run uses
400 connection slots and bounded package/test parallelism. The existing Import Workload restriction
assertion now allows only the newly approved .env picker. No assertions were
removed to accommodate a product failure. One unchanged observability timer test
failed in an earlier full frontend run, then passed focused and in a complete
rerun; final results below refer to the final source.

## Main baseline failures

Full baseline source includes all repository fixtures; earlier missing-file
harness failures are not used as evidence of a main defect. The backend tree and
its cross-directory inputs (`inlineTemplates.ts`, old E2E scripts, catalog) are
unchanged by this task. Full Go stays red; no backend tests were weakened or
production code expanded to repair these unrelated failures.

| Package (under `backend/`) | Representative failure on main |
|---|---|
| internal/adapters/postgres | TestBaselineEnforcesCurrentConstraints; TestVersionSixCapturesPopulatedFiveAndProtectsSnapshots; TestUpgradeBackupRestore; TestVersionSevenPreservesConnectionsAndAdmitsGit; TestUpgradeHTTPLoginPreviewDeploy |
| internal/adapters/templatesources | TestInspectRejectsUnsafeSourcesWithoutFetching |
| internal/application/catalog | TestListDefinitionCapabilities_PlatformPairsExistingClusterAndWorkload |
| internal/application/configuration | TestRenameWithReferencesPreservesAliasesManyToOneAndOtherFields |
| internal/application/deployment | TestCredentialBackedTarget_RemovalAfterRestart |
| internal/application/pending | TestRequiredRendererMissingTieAndUnavailableFailBeforeResources |
| internal/application/preview | TestPreview_NoRuntimeMutation |
| internal/application/provisioning | TestProvision_PostgresOutputContractEquivalentAcrossPlacements |
| internal/bootstrap | TestRealProfileAcceptsOnlyTheFullRealWiring |
| test/conformance | TestPlannerMatchesChallengeFixtures; TestProductPlannerRejectsCriteriaLessDefinition |
| test/e2e | TestUC12UC16HTTPDraftFlow |
| test/integration/v2cleanup | TestDeliveryCleansAppliedTargetsWithoutLegacyEnvironmentBinding |
| test/integration/v2run | TestTheScenarioScriptRecordsExactlyTheAssertionsTheRunnerRequires |

## Real browser and runtime

A dedicated Docker Console used real PostgreSQL, scoped Vault and existing kind
Fleet/Git configuration, seeded through ordinary APIs. All workload/key authoring
and Preview/Deploy went through the real Console; no mocked backend or runtime.

- Entered 36 variables covering literals, Application Variable, Secret,
  resource output and another workload's Service. Empty, =, Unicode, leading @
  and a true multiline value retained their semantics.
- Declared two PostgreSQL dependencies with distinct aliases; alias rename
  updated both affected bindings/text without provisioning at authoring time.
- One bulk confirmation created two synthetic Secrets. Saved/reopened/exported
  content contained references; resource secret outputs used metadata and
  remained references throughout.
- Saved both workloads/resources before one Environment Preview/Deploy. The
  Operation succeeded; live Kubernetes reads verified distinct resource targets,
  Service address, shared Variable value, literals, and Secret delivery without
  printing Secret bytes.
- Reopened after Deploy and backend restart with all 36 names in authored order.
  Manifest env ordering stays renderer-owned under UC-16 BR-34/GW-09.
- VI/EN, 1512×980 and 1280×900 checked. File download/sample and conflict merge
  passed; cancelling imported edits left stored workload unchanged. Saving the
  same multiline content produced a no-op Preview, without another Deploy.
- A separate browser regression made a missing resource reference valid by adding
  its dependency, checked that the new binding reached UI, then cancelled and
  verified zero workload mutations. Keyboard arrow navigation/focus and read-only
  secret-output classification also passed on the retained review draft.
- Early browser attempts had locator/wait assumptions corrected (textarea/native
  select labels and loaded rows); the successful assertions above were rerun.

UI reference: AppShell, tokens/app styles, shared Button/Dialog, the current
Workload editor and Settings. Compared the original product editor in browser
with final screenshots. Kept shell/navigation and Name/Value/source picker;
new copy is limited to tabs, actions, errors and the independent Secret commit
boundary, with syntax help collapsed. Selection checkboxes align with names;
no whole-page horizontal overflow at either target width.

## Cleanup and retained review state

The first Application cleanup stopped at the foreign-child namespace guard:
our bootstrap Role/RoleBinding were still present. After checking their owner
labels and UIDs, removed exactly those setup objects with UID preconditions and
retried product Application deletion. Cleanup succeeded; a successful Kubernetes
API lookup confirmed namespace absence. No namespace guard was bypassed.

The dedicated full-check namespace was deleted with an owner-label check and
namespace UID precondition after every test process finished; a successful API
lookup confirmed absence. Only this run's test-image references were removed;
the feature images, shared proxy, unrelated stacks and cluster were retained.

The feature Docker stack and its own PostgreSQL/Vault/cache volumes remain for
acceptance; runtime override/start instructions and private evidence are outside
Git under `~/.local/state/issue72`. A separate undeployed review draft is retained
with synthetic keys. Its link is sent privately in the handoff, never written to
the public issue or this record. The active tailnet daemon was confirmed Running
in the host distro; Windows HTTP access to the same feature route also returned health 200. The exact Tailscale route was opened from this host in browser with same-origin API checks; this is not a second-device/ACL test.

No AWS/cloud run, other cluster deployment, main merge or main deployment was
performed. Main deployment and issue closure await user acceptance under AGENTS.
