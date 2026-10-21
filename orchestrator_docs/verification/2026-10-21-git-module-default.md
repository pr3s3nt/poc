---
id: VER-20261021-GIT-MODULE-DEFAULT
artifact: verification-evidence
status: evidence
last_reviewed: 2026-10-21
---

# Git Terraform module publication and default source

## Outcome

Public repository [pr3s3nt/terraform-modules](https://github.com/pr3s3nt/terraform-modules)
was created with seven unchanged module directories, a usage README and tag
`v1.0.0`, resolving to `6b991b16c39ac8485c74fa6756cf3223a129c8c9`.
`git ls-remote` confirmed main and the annotated tag's peeled commit.

The product change starts new/reset/successful Definition drafts in Git, clears
completed Git inspection state, and preserves stored pins on edit. Registry
inspection retains release identity/executor outputs only for exact trusted
release content and matching Type. This addresses the discovered EKS identity/
executor-output and namespace creation-baseline mismatch; arbitrary changed Git
code does not inherit release module semantics.

## Checks

- Focused registry tests PASS, including all seven module copies, exact Git pins,
  executor outputs, registry restart/cache, Type mismatch and modified content.
- Catalog, Terraform executor and planner package tests PASS; `go build ./...` PASS.
- Frontend impact tests: 131 tests across eight component/metadata files PASS.
  Fixtures now explicitly select registered modules where that is their scenario;
  source-default tests exercise the actual initial Git state. Existing Kubernetes
  tests were updated to enter the catalog's Create view before selecting Types.
- Typecheck, changed-file ESLint and production build PASS.
- Real browser with temporary HTTPS Git + real HTTP/catalog/registry PASS:
  tag inspection, immutable commit/digest, create, fresh Git draft/reset, reload,
  edit and CAS; registered selection remains available explicitly.
- v3 browser PASS: tabs, form/JSON, mapping names, companion params/flags,
  criteria/copy/matching, create/edit/CAS/reload and keyboard/mobile. Cancel
  returns to Git; later registered-module authoring explicitly switches source.
- Public GitHub browser/API PASS for all seven published modules: exact commit
  and release content digest, Git origin/pin, release-equivalent module ID,
  executor outputs. EKS default Git form inspect/output mapping PASS.
- Documentation checker and `git diff --check` PASS.

Initial failures from tests assuming an automatic registered selection after
Save/reset were corrected to choose that source explicitly. The public-source
fixture initially lacked the Redis Resource Type; it now registers the normal
Redis contract in its isolated catalog before inspection. No assertions were
removed to obtain the passing results.

## Limits and existing work

Whole-frontend `npm run lint` remains failing on 26 existing errors in design
prototype scripts, `ec2-role-real.mjs`, and the user's untracked
`frontend/test/e2e/deploy-kind-demo.mjs`. Those files were left unchanged;
changed-file lint passed. Vite reports the existing large-chunk warning.
React tests also emit existing act warnings.

No fullsuite checkpoint, PostgreSQL integration, Terraform apply, Kubernetes or
AWS mutation was run: scope was publication, module inspection and Definition
authoring. Existing deployment scripts were adjusted to explicitly select
registered modules; external deployment scenarios were not rerun. Browser
catalogs/caches use temporary fixtures, removed by the harness. Packaged modules
and original execution/cleanup pins are retained; existing Definitions/resources
were not migrated. The user's untracked E2E file remains unmodified and unstaged.
