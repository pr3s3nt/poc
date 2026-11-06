---
id: VERIFY-WORKLOAD-DEFAULTS-MAIN-MERGE-20261106
artifact: verification-record
status: current
last_reviewed: 2026-11-06
---

# Issue #37 — integration into main and retained Console upgrade

User approved merging `features/bo_sung_default` into main after the authoring
workflow and guide follow-ups. Feature head before integration was `e544fee0`;
remote main advanced from the original baseline to `e3903609`.

## Integration

Commit `2f6f5d03` merges current main into the feature. Two documentation index
conflicts preserve both sides. Main's ADR-038 remains readable IDP names; the
feature defaults decision is renumbered ADR-039, with matching IDs/links. No
released schema/migration or default values were changed for integration.

The existing browser scenario now consumes the API's namespace identity and the
new readable Fleet repository identity with a stable Application ID label.
Definition keys use each application's unique suffix. Import waits for the new
parse response and exact template hash before Save, then asserts the saved pin.
It can resume an already successful legacy Deploy after independent native checks.
The scoped namespace-access helper reads published Environment identity for new
IDP names, retaining its legacy-only fallback and all ownership/rules guards.

These runner corrections followed two observed acceptance failures: legacy Deploy
succeeded but a historical key-versus-ID assumption missed its Fleet GitRepo; a
subsequent import clicked Save while previous parsed-status text was still visible.
No product guards or assertions were removed. The saved pin and native identity
are now checked explicitly; the same owned application was completed and cleaned.

## Checks before merge

- Affected environment/template domain, configuration/review, planning/workloadspec,
  renderer, HTTP, application creation/delivery and Fleet Go packages pass.
  `go build ./...` passes.
- 138 Workload UI tests (12 files), typecheck, lint and production build pass.
  Existing PlatformSettingsPage lint and bundle-size warnings remain.
- 78 bootstrap tests pass with zero skips. E2E/helper Node syntax, documentation,
  diff and migration immutability guards pass.
- Real PostgreSQL, REQUIRE_POSTGRES=1: template snapshot/false/zero JSONB reopen,
  execution reopen and concurrent readable names all pass (three tests, no skips).
- Real owned-kind adapter run passes in 171.04 seconds: non-root/read-only Nginx,
  two writable emptyDirs, PDB pruning, shared PVC UID/data reuse, StatefulSet
  scale-down/restore/removal claim retention and explicit owned cleanup. Its
  namespace is independently verified absent.
- Real browser/Fleet flow on the integrated code passes: legacy Deploy, no-op
  Save/Preview, pinned-template adoption, two Ready replicas, explicit false token/
  zero history, template resources, preserved Deployment UID, effective artifacts,
  Preview resource-version purity and PDB enable/prune. Product removal/Application
  deletion pass; namespace and GitRepo absence are independently verified.

Per the current AGENTS workflow while Product CI is red, the full acceptance gate
was real kind execution. Full unit-suite failures from the preceding runs remain
recorded; this merge does not relabel them green or infer CI success. No AWS test.

## Merge and operational handoff

Merge commit `18b902c9` is pushed to main, with machine-user attribution and
`Refs #37`. The feature branch is deleted locally/remotely. Its isolated Compose
services and dedicated Tailscale Serve route are stopped; volumes and private
receipts remain, and shared images/services/routes are preserved.

The official backend/frontend Dockerfiles built the integrated product tree;
main-18b902c9 image tags point to those artifacts. They were loaded into the
existing kind-k8s-4f node and deployed using the checked-in Console manifests.
Only documentation metadata follows the integration tree; product code is identical.

Before owner setup, Console writers were stopped. A private logical PostgreSQL
archive was restored into a separate verification database, compared successfully
and dropped. Vault was quiesced while its files, generated credentials, state key,
and backend state/cache were archived privately; archive integrity was checked.
The temporary read-only backup Pod was removed with its original UID precondition.
Owner setup completed, backend/frontend/Vault are Ready, and the original platform
PVC UID and original Application count are preserved. No database reset or released
migration/reference edits occurred.

The retained Console's exact Tailscale route was opened with real authentication
before/after upgrade; an initial request during forward reconnection failed and
subsequent browser/API checks passed. Private network addresses and credentials
are excluded from this public handoff. The final tested Console link is supplied
in the user report. Keep the existing cluster and forwards running; restart and
upgrade procedure: [Console manifests](../../deploy/kind-k8s-4f/README.md).

Private logs/receipts remain outside Git: merge-kind, merge-acceptance/flow,
merge-cleanup, merge-postgres, main-upgrade, main-upgrade receipt/backup archives,
and main-acceptance. Their first failures and subsequent successful assertions
are retained. Post-upgrade acceptance results are recorded below when complete.

## Post-upgrade acceptance — passed

The same positive browser/Fleet scenario also passed against the retained main
Console on kind-k8s-4f: legacy Deploy and no-op Save/Preview, adoption of pinned
1.1.0 defaults, two Ready replicas, unchanged Deployment UID, read-only/non-root
Nginx with two writable emptyDirs, false token and zero history, effective Preview
artifacts/provenance, PDB enable/prune and product removal. No response interception
or direct workload apply substituted for the business flow. Product Application
deletion passed; its namespace and Fleet GitRepo were independently verified absent.
Receipt flags `acceptancePassed` and `cleanupPassed` are both true.

The merged commit's standard template directory was separately inspected through
the real Git source form. Its Git record is AVAILABLE at commit `18b902c9`, with
unchanged template content digest. The final browser check at
`2026-11-06T11:00:51Z` confirms the original Application remains, the new Backend
1.1.0 form resolves muted 100m defaults, Service offers distinct choices, and the
backend-policy selector is absent. It used an unsaved form under the original
Application; its existing workloads/draft version were compared unchanged.
The Console application list and relevant same-origin APIs load without browser
errors. This checks access from this host through the exact Tailscale route,
not every tailnet device/ACL.

The main Console/forwards and private backup remain running/retained. The old
feature URL is intentionally stopped after merge; its Docker volumes were not
deleted. No other user stack, tmux session, cloud resource or application was
removed. Documentation-only verification changes after the merge do not alter
the deployed product image or template content.
