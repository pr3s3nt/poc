---
id: VERIFY-20261021-DEFINITION-V3
artifact: verification
status: evidence
last_reviewed: 2026-10-21
related: [UC-03]
---

# Issue #8 — Resource Definition v3

Branch `iam_role`, base and fixed design reference
`32db49f5cb1df6a855e55241e26962e1cb0adec2`. Read issue body (no comments),
UC-03 sources, prototype README; inspected all three images and exercised the
preserved HTML mapping/Form–JSON/companion controls in Chromium. Prototype files
were not changed. Existing untracked `frontend/test/e2e/deploy-kind-demo.mjs`
was left untouched; IAM-role/backend code was not changed.

## Outcome

Two mounted task tabs with keyboard navigation; advanced mapping/provider and
companion editors collapsed initially, existing variables/mapping/provision
converters retained. Five-field AND/OR rule projection for Definitions, exact
criteria literals and wildcard/zero-case/incomplete-condition distinctions.
Independent copy with overwrite confirmation; collapsed matching/help, scope
summary and full draft review. Hidden panels retain signature/session/late-reply
invalidation. Renderer keeps its previous projection. No new API/DTO, pin,
credential, CAS or deployment behavior. Canonical UC-03/UI/realization plus
traceability/current state/code map updated.

## Focused checks

- `npm test -- --maxWorkers=2` with these eleven frontend files: **211 PASS**:
  `ResourceDefinitionsPage.test.tsx`, `DefinitionV3.test.tsx`,
  `CriteriaEditor.test.tsx`, `MatchingPreviewPanel.test.tsx`,
  `RenderingTemplatesPage.test.tsx`, `TypedDefinitionForm.test.tsx`,
  `RegistrationStates.test.tsx`, `MetadataValidation.test.tsx`,
  `GitModuleSource.test.tsx`, `moduleMapping.test.ts`, `typedConversion.test.ts`.
  Includes existing required/type/secret/provider/account/source/CAS/unknown-save
  outcome assertions and shared renderer tests. One earlier parallel run hit a
  five-second EKS authoring test timeout under worker contention; reducing worker
  concurrency passed the complete affected selection without changing assertions
  or test timeouts. Final CSS-only scope-summary placement reused these tests.
- Typecheck and production build PASS; existing large-chunk advisory remains.
- Changed-file ESLint PASS (ten changed/new TS/TSX/MJS files), max warnings zero.
  Full-directory lint was also attempted: preexisting errors in prototype
  `advanced.js`/`serve.mjs`, `ec2-role-real.mjs`, and user-owned untracked demo.
  No unrelated cleanup was made to get a green global lint.
- `python3 scripts/check_docs.py` and `git diff --check` PASS. No PlantUML changed;
  inspected current UC-03 sequence/VOPC and operation/module contracts, which
  retain the same operations/participants/transactions.

## Browser acceptance

```bash
cd backend
go run ./test/integration/gitmodulebrowser \
  -scenario ../frontend/test/e2e/resource-definition-v3-local.mjs
```

Real product HTTP/auth/catalog/registry, actual hardened Git fetch against an
owned temporary HTTPS git-http-backend with annotated tag and fixture CA,
production frontend bundle; catalog in memory. No intercepted/fake inspect,
matching or mutation response. Type creation is scoped to this temporary catalog.

PASS: Type with unequal input/module names (`message` → `label`) and module/Type
output names (`name` → `title`), Git inspect and immutable commit, mapping JSON
errors retained across tabs, typed fallback, companion add/edit/delete/params/
both flags/Form–JSON, exact submitted review payload, AND/OR cases, independent
copy cancel/confirm, real matching and hidden-result invalidation, create201,
reload and restore, concurrent real CAS winner → stale409 → explicit reload →
edit200/new version, keyboard tabs and no document horizontal overflow at390px.
Shared Rendering Templates browser smoke checks the original criteria projection,
multiple cases, fixed weights and matching-context controls. No uncaught page errors.

An initial exact-label browser locator exposed unstable wrapped JSON/select
accessible names; explicit aria labels corrected the UI. Browser also verifies
explicit `match_dependents: false` in the submitted document and unchecked state
on reload; existing Go `omitempty` removes false from response JSON, so response
assertions check that unchanged server representation separately. No backend
normalization was changed.

Local screenshots: `/tmp/issue8-v3-browser/{configuration,matching,advanced,mobile}.png`.
Harness defers close HTTP/TLS and remove task-owned repositories/cache/CA;
Chromium closes in finally. No user tmux/session or existing service was stopped.

## Review and limits

`cd frontend && npm run dev` serves
`http://127.0.0.1:5173/ui/platform/resource-definitions`, proxying API to
`http://127.0.0.1:8080` by default; use `ORCHESTRATOR_API_ORIGIN` for another API.
Review either a verified module/new Definition or open an existing Definition
with Sửa. Advanced settings are in Cấu hình; criteria/copy/matching/help are in
Điều kiện áp dụng. Backend still checks every Save/inspect/preview independently.

No fullsuite, unrelated deploy/app/jobs/PostgreSQL/Redis/AWS/Kubernetes acceptance,
Terraform apply, database persistence or cloud verification was run. Backend
product code is unchanged; browser compiles/uses its existing focused harness.
