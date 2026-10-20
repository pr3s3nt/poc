---
id: VERIFY-20261020-GIT-MODULE
artifact: verification
status: current
last_reviewed: 2026-10-20
related: [UC-03, ADR-014]
---

# Issue #7 — direct Git module authoring

Branch `iam_role`, base `c3c8cfc`; issue https://github.com/pr3s3nt/poc/issues/7.
User authorized direct PE/Admin HTTPS source authoring without repository/module
allowlists and focused tests/E2E only. Existing IAM-role implementation retained;
user-owned untracked `frontend/test/e2e/deploy-kind-demo.mjs` untouched.

## Outcome

Git URL/path/tag or commit → explicit inspection → immutable commit/digest and
capability metadata → typed inputs/output/provider mappings → Definition create
or CAS edit. Safe origin retained in bundle contract JSON. No column migration.
Full digest hidden in technical details; installed module choice retained.
Inspection persists only registry/cache, never Definition/criteria/state or cloud.
Legacy source policies remain readable; direct source preparation needs no entry.
Git child modules require exact commits and are inspected/cached as before.

## Focused checks

- Full affected registry and tfmodule domain packages: PASS.
- Catalog/HTTP affected tests selected by `TestInspectGit|TestInspectTerraformModule|
  TestGitCLI|TestTransitive|TestListDefinitionCapabilities|TestDefinitionCapabilities|
  TestUpdateResourceDefinition`: PASS. Packages with no matches were not counted.
- Race selection `TestInspectGit|TestInspectTerraformModule|TestUpdateResourceDefinition`
  in registry/catalog/HTTP: PASS, including existing concurrent Definition CAS.
- `TestModuleRegistryBootstrap`: PASS; production command and focused browser
  harness build, affected registry/catalog/HTTP/harness vet: PASS.
- Frontend `ResourceDefinitionsPage`67, `MetadataValidation`32,
  `GitModuleSource`2:101PASS together; `TypedDefinitionForm`6PASS reused because
  its tested typed-input code did not change afterward.107distinct tests, no
  overlapping reruns added to the count. Git source tests repeated after a ref
  cleanup adjustment:2PASS. Typecheck/build PASS; generated bundle remained
  `index-CZyldeD8.js` across final rebuild, so browser result was reused.
- Changed-file ESLint with `--max-warnings 0`: PASS. Whole-repository lint was
  attempted and fails on preexisting untracked demo unused `expect`, and tracked
  `ec2-role-real.mjs` browser globals `document`/`window`. Neither was edited.
- Docs checker, diff whitespace and three changed PlantUML sources rendered.

## Browser execution

`cd backend && go run ./test/integration/gitmodulebrowser` runs real hardened
Git CLI against a task-owned HTTPS git-http-backend fixture with an annotated
`v1.0.0` tag, real HTTP/auth/catalog/registry and production Console bundle. It
uses an in-memory catalog and a fixture CA wrapper, with no source policy entries.
No Terraform CLI/provider/DB/cluster/cloud mutation or fake inspect response.

Main flow PASS: Type without installed module → source inspection → commit/digest
assertions → digest hidden by default → generated variable/output mapping →
Definition201 → reload/open → values/mappings restored → CAS edit200/version2 →
reload → installed module selection. An initial runner failed on an exact label
locator for a wrapped select; locator fixed to the actual combobox name. The
subsequent complete main flow passed. No unrelated app/job/deploy acceptance rerun.

Go defers close browser harness HTTP/TLS servers and remove its owned temporary
repositories/cache/CA; Node closes Chromium in finally. No user sessions/processes
or retained environments were stopped.

## Limits

Only public/unauthenticated HTTPS Git is supported; private credentials/SSH are
not introduced. Provider pins/content restrictions/readiness support remain as
before. No Terraform apply, new-provider lifecycle or PostgreSQL persistence
claim. Durable registry storage plus `-terraform-module-cache-dir` is required for
reuse across restarts; registry restart reuse is covered by the focused unit test.
No fullsuite, external AWS/Kubernetes, resource upgrade/destroy or unrelated E2E.
