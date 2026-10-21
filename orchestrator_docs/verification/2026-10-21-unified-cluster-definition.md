---
id: VERIFY-20261021-UNIFIED-CLUSTER-DEFINITION
artifact: verification-report
status: current
last_reviewed: 2026-10-21
---

# Unified existing-cluster Definition authoring

User approved merging existing-cluster into the common Resource Definition
catalog and Create/Edit view. UC-03 UI3-05 owns the UI requirement. This task
changes frontend and documentation only; API, matching and lifecycle unchanged.

## Outcome

One catalog and Create/Edit entry; k8s-cluster offers Terraform and existing
cluster using server capabilities. Existing-cluster selects its own READY
Internal account, with common identity, criteria tabs, payload review and
Save/CAS/explicit reload. Module/region/variable controls are hidden for this
method. Each method retains its account/inputs, including mounted Git source
fields. Infrastructure Accounts remains separate. Removed the obsolete separate
cluster section; retained and adapted its regression cases in
`ExistingClusterDefinition.test.tsx`.

## Checks

- ResourceDefinitionsPage: 67 tests PASS, including shared catalog, create/edit,
  metadata/account loading, stale/unknown outcome and explicit reload.
- ExistingClusterDefinition: 6 tests PASS, including Internal account filtering,
  exact payload, expected version, stale hold, method/tab/catalog draft retention
  and Git fields across method switches.
- Shared affected suites: DefinitionV3, TypedDefinitionForm,
  RenderingTemplatesPage, CriteriaEditor and MatchingPreviewPanel: 79 tests PASS.
  Repeated focused checks after mounted-module correction also PASS.
- Typecheck/production build PASS; Vite retains its existing large-chunk warning.
- ESLint for changed frontend files PASS. Full `npm run lint` was attempted and
  reports 26 preexisting errors in `design/resource-definition-v3/advanced.js`,
  `design/resource-definition-v3/serve.mjs`, `test/e2e/ec2-role-real.mjs` and the
  user-owned untracked `test/e2e/deploy-kind-demo.mjs`; those files were untouched.
- `python3 scripts/check_docs.py` and `git diff --check`: PASS.
- Repository search confirms no remaining current source import of the removed
  component; code map updated. Historical verification retains its original name.

Headless Chromium check PASS: create/edit, method drafts, shared tabs/catalog,
Internal account, no module/region controls, desktop 1440/mobile 390 and no page
errors or document horizontal overflow. HTTP was mocked in this browser check;
it does not prove backend persistence or deployment. Local runner:
`/tmp/unified-cluster-browser.cjs`; screenshots inspected at
`/tmp/unified-cluster-desktop.png` and `/tmp/unified-cluster-mobile.png`.
The temporary Vite process was stopped after verification.

No backend/product code changed, so Go tests/build were not rerun. No fullsuite
checkpoint, PostgreSQL, Kubernetes, AWS or provider mutation was run: these are
outside this UI task. Existing user-owned untracked demo script was preserved
and excluded from the commit.
