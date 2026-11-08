---
id: VERIFY-2026-11-08-RETAINED-PROGRESS-CATALOG
artifact: verification
status: current
last_reviewed: 2026-11-08
---

# Retained progress and Catalog UI — issue #47

User approved correcting the frontend deployment detail showing 6/7 despite a
successful batch and hiding Catalog authoring from Console. Work continues on
`features/organization_seed`; no main merge or new application deployment.

The graph contained both workloads but the current Deployment owned only the
frontend snapshot. Query assembly now supplements progress-only facts from
successful producers of the exact base Deployment Set, following immutable
ancestry through unchanged module intent. It requires explicit RETAIN/BIND,
matching tenant/Environment/target generation and an exact READY snapshot with
a manifest digest. Missing/conflicting evidence remains UNKNOWN. The current
run's snapshots win and its Workloads list is unchanged; neither current Pod
status nor overall SUCCEEDED makes a node ready. No schema/write/refresh path
was added.

Catalog has no sidebar entry, source create/edit/refresh form, or Environment
release-selection controls. Direct legacy Catalog routes keep source/releases
and usage readable. Existing Environment selections are read-only; empty Catalog
panels are hidden. Definitions/Git authoring, backend APIs, stored pins/releases
and resolution remain intact. Shared UI design and existing shell/styles were
the visual reference; browser before/after confirmed the unchanged shell with
the Catalog entry/form removed. No new explanatory product text was needed.

## Checks and runtime

- Focused Go query/progress tests pass, including exact baseline READY, missing
  proof, failed snapshot, wrong snapshot owner, tenant/generation mismatch,
  changed/unchanged ancestry, ambiguous ancestry, later/unrelated success and
  current-run failure precedence. `go build ./...` passes.
- 22 focused Console tests pass: shared navigation, legacy Catalog read-only
  source/releases, existing/missing Environment pins and Settings regressions.
  Typecheck/lint/build pass; lint retains one existing PlatformSettings ref warning.
- Dedicated feature Console images were updated after retaining a private database
  backup. No sample app Deploy, Fleet/Pod rewrite, main upgrade or storage cleanup
  occurred. Runtime source pins were not re-inspected or changed by this query/UI
  task; existing renderer-pinning constraints still apply to future deployments.
- Real browser opened the exact reported deployment URL through Tailscale:
  **7/7**, backend **RETAINED**, frontend READY, current-run Workloads still only
  frontend. It also opened the legacy Catalog URL: GET loaded, sidebar entry and
  create inputs absent. Evidence/screenshots stay outside Git. Existing Catalog
  rows were absent in this stack; read compatibility for populated rows is covered
  by component tests, not a fabricated live source.
- Full frontend: 981 pass / 2 fail (983 tests). The same two static legacy
  scenario checks (`terraform-v2-labels`, `v04-internal-full`) fail unchanged
  on the retained pre-rollback baseline `5f5715c2`; their missing Score label/
  scenario IDs predate this task. The earlier main comparison is recorded in
  issue #49. No full-green claim. The failed historical frontend deployment
  still returns FAILED after the fix; empty Settings has no Catalog panel.
- Full backend on an owned kind namespace, complete repository and Go/git/Redis/
  Node/Bash/Terraform tooling: 15 packages fail, all included in the prior main
  comparison's failing packages; no additional named failures versus that main
  log. These include existing rendering/fixture/provisioning contract failures.
  PostgreSQL was not configured for optional suites in that Pod; the changed
  query/progress tests use isolated memory facts and pass independently. The
  first run lacked Terraform CLI; its extra named bootstrap failure reproduced
  on main, then passed on both after the tool was supplied. Only the corrected
  complete-tooling run is used for the final comparison. Namespace UID/run label
  was checked before deletion and namespace absence verified. No full-green claim.
- This record does not change
  the stopped Backstage guest/catalog business acceptance or deferred rollback
  scenario 2.

Feature stack and endpoint forwards remain active. The private resume script
records the new images; public documentation does not contain Tailscale URLs,
credentials or database dumps. Main merge awaits user acceptance.
