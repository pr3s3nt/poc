---
id: IMP-WORKLOAD-ENV-EDITOR
artifact: implementation-handoff
status: current
last_reviewed: 2026-10-24
related: UC-12, UC-16, UC-05, UC-06, ADR-020
---

# DEVEX-ENV — Bàn giao triển khai bảng biến môi trường workload

Issue triển khai: [GitHub #13](https://github.com/pr3s3nt/poc/issues/13).
Issue liên kết commit thiết kế `7845786`; đọc HEAD của nhánh để nhận bổ sung mới nhất.

## Outcome và trạng thái

Developer khai báo biến bằng một bảng Tên biến → Nguồn → Giá trị/tham chiếu
ngay trong container; tạo key hoặc dependency còn thiếu tại chỗ. User đã duyệt
luồng này ngày 2026-10-24 và yêu cầu bàn giao đủ để một coding agent tự thực hiện.
**Đây là thiết kế được duyệt, chưa có implementation hoặc acceptance mới.**

Nhánh tài liệu: `features/refactor_dev_ex`, đã cập nhật `main` tới `8f4d66f`
ngày 2026-10-24. Commit đó bổ sung i18n Việt/Anh; không implement trên base cũ
`384dee6` hoặc bỏ qua ADR-020. Khi nhận việc, fetch nhánh tài liệu và đọc HEAD;
không chỉ checkout main vì thiết kế này có thể chưa được merge.

## Nguồn chuẩn — đọc theo thứ tự

1. Root [AGENTS.md](../../AGENTS.md), [index](../INDEX.md),
   [shared UI](../architecture/ui/README.md), [ADR-020](../architecture/decisions/ADR-020-console-message-catalogs.md).
2. [UC-16 specification](../usecase/UC-16/specification.md): MS-03–05,
   BR-02/04/09/15–25; [UC-12](../usecase/UC-12/specification.md): BR-24–28.
3. [Screens](../usecase/UC-16/ui/screens.md), [states](../usecase/UC-16/ui/states.md),
   [API mapping](../usecase/UC-16/ui/api-mapping.md).
4. [OC-12/16](../architecture/contracts/operation-contracts.md): create-only,
   Score mapping, safe literals/provenance, independent commits.
5. [UC-16 realization](../usecase/UC-16/realization.md), sequence/VOPC linked
   from its context; [UC-12 realization](../usecase/UC-12/realization.md).
6. [Browser reference](../verification/2026-10-24-devex-env-design.md),
   [traceability](../traceability/matrix.md#devex-env--approved-design-awaiting-implementation),
   [gap IMP-034](deviations.md#imp-034--unified-workload-environment-editor).

Không cần xin lại quyết định năm nguồn, giá trị thường trực tiếp, inline create,
độc lập commit hay Việt/Anh. Agent tự quyết định component extraction, thuật
ngữ nội bộ, cấu trúc provenance nhỏ nhất và test fixtures trong contract này.
Không tự giảm scope thành frontend-only hoặc bỏ literals/inline create.

## Phạm vi và ranh giới đã chốt

- Năm nguồn: Giá trị nhập, Biến của ứng dụng, Bí mật, Tài nguyên, Workload khác.
  Giữ nhiều mapping cùng key/output, alias và isolation mỗi container/app/env.
- Giá trị nhập thuộc container, không tự sinh key UC-12. Secret luôn reference.
- Tạo biến/bí mật ngay dialog qua create-only API; shared key commit riêng,
  Cancel workload không xóa key. Existing shared-key edit/rename/delete ở Settings.
- Chưa có kho: initial selection tại dialog; đổi kho đã có qua Settings.
- Thêm dependency ngay dòng chỉ sửa parent draft; chọn output tường minh.
- Ordinary literal mới được round-trip qua import/save/deploy/reopen/restart/
  rename. Giữ no-readback của legacy unknown literals bằng trusted provenance.
- Giữ Score contracts, Type/class/shared ID, matching, routes, safe error envelope,
  CAS/BUSY/stale, Preview → Deploy và secret delivery hiện hành.
- Không thêm .env import, bulk paste, expressions/template interpolation mới,
  auto-binding tất cả outputs, auto-rollout, cloud/provider feature, mobile
  redesign, new visual identity hoặc một workload-creation wizard khác.

## Code cần kiểm tra và thay đổi

| Vùng | Entry points / việc cần làm |
|---|---|
| Editor | `frontend/src/features/workloads/WorkloadEditorPage.tsx`, `bindings.ts`, `ApplicationKeyPicker.tsx`, `ClassControls.tsx`, `api.ts`: thay checklist/Other sources, nguồn literal, stable row identity, shared dependency subform, import/buildScore/fromScore lossless |
| UC-12 UI | `frontend/src/features/configuration/api.ts`, `SettingsPage.tsx`; `features/environment/SecretStoreSelection.tsx`: reuse validation/store selection, thêm create-only client và inline form; không invoke upsert từ create |
| Shared UI/i18n | `frontend/src/shared/ui`, `styles/app.css`, `shared/i18n`, `public/locales/{vi,en}.json` và manifest: reuse styles/dialog, mọi text mới qua t(), đủ placeholders |
| Workload backend | `backend/internal/application/workloadconfig/{service,reconstruct,rewrite}.go`, HTTP workload handlers: ordinary strings, strict reference classification, safe readback and rename |
| Configuration backend | `backend/internal/application/configuration/service.go`, `backend/internal/delivery/http/{configuration,server,apierror}.go`: CreateKey + POST route + typed errors/CAS |
| Provenance/lifecycle | Domain/persistence adapters, pending preview/deploy, successful per-workload commit, snapshots/current-set reconstruction: exact-value/version association across success/partial/restart; no client marker bypass |
| Regression surfaces | configuration rename, pending no-op/stale preview, deployment review redaction, Score escape/parser/renderer, resource-output secrecy, schema/snapshot compatibility if metadata added |

Current `Put` updates an existing same-kind key; catalog-check + PUT is not a
create-only solution. Current `validateBindings` rejects literals and
`ReconstructScore` hides them because historical values may contain secrets.
Do not solve new literals by flipping `allowLiteral` globally. Public planner
Delta/module shapes and direct Score API semantics must stay compatible; private
provenance storage can be designed without changing Score wire fields. If a DB
change is required, update schema/ERD and additive persistence validation first;
missing historical provenance means unknown, never inferred safe or reset.

## Thứ tự thực hiện

1. Inspect current UI in browser and compare reference, read code/tests; keep
   unrelated changes. Update any affected schema/internal design before code.
2. Implement and test create-only backend; ordinary literal admission and trusted
   readback through the complete lifecycle, preserving reference validation.
3. Build reusable unified rows and inline key/dependency/store interactions;
   integrate import/Edit, drafts, focus, error/scope guards and both languages.
4. Add focused component/API/persistence/lifecycle regressions below; update
   superseded literal-rejection/checklist tests to approved behavior while
   retaining negative secret/legacy coverage.
5. Run local browser acceptance, review screenshots against existing shell and
   concise copy, fix failures, run required checks, update code-map/gap/current
   state/traceability with actual evidence, commit task-only and push/PR.

## Acceptance matrix

| ID | Scenario / required assertion |
|---|---|
| E01 | One table per container; add/change/remove each of five sources; changing source retains name and excludes old source payload; delete row has no key/dependency mutation |
| E02 | Literal `debug`, empty string, spaces, dollar/escaped placeholder: import → save → reopen → local fake deploy → current-set Edit → backend restart keeps exact value/semantics; NUL/non-string/malformed reference rejected |
| E03 | Existing Variable/Secret keys, container aliases and two rows for one key survive Edit/import; no implicit select on catalog refresh; same name across containers allowed, duplicate within container blocks Save |
| E04 | Create Variable and Secret without leaving editor; duplicate same kind and concurrent create do not overwrite; explicit select-existing; key remains after workload Cancel/failed Save; known/unknown response handling |
| E05 | Missing store can select/save READY store inline, then create Secret; no stores gives actionable state; store saved/key failure distinction; no secret in Score/readback/Preview/evidence |
| E06 | Add PostgreSQL or another registered Type inline with typed params/class/shared ID; choose host and secret output on separate rows; second container reuses dependency; Cancel adds nothing; no provisioning before Deploy |
| E07 | Resource alias/type/output removed, key renamed/deleted, Service port removed or target pending delete: retained invalid rows and explicit correction; no retarget or silent row loss |
| E08 | Service source same env only; exclude self/pending delete; select valid port; preserve resource/Service mappings through UC-12 rename |
| E09 | Validation/catalog/network errors, BUSY/stale, committed-key/reload failure; late replies after row removal/scope switch cannot mutate other row/scope; no automatic POST/PUT replay |
| E10 | Old unproven literal cannot be read via list/Edit/Preview/rename or fake provenance; new accepted literal remains editable after deploy/restart; partial deploy only promotes successful workload provenance; direct-API value change invalidates it |
| E11 | Save/config mutation only pending; older Preview rejected; no-op remains no-op; routes, class/shared IDs, advanced Score and secret redaction regressions pass |
| E12 | Real product UI in Vietnamese and English at 1512×980 and 1280×900; labelled controls, dialog keyboard/focus return, no overlay/overflow; new catalog keys complete in every shipped language |

E04/09/10 negative/concurrency coverage belongs in focused tests; browser happy
path need not intentionally execute every failure. No paused cursor simulation,
video approval or user review checkpoint is needed for this issue.

## Validation and evidence scope

Follow AGENTS required checks for changed backend/frontend:

```bash
cd backend
go test ./...
go build ./...
cd ../frontend
npm ci
npm run typecheck
npm run lint
npm test
npm run build
cd ..
python3 scripts/check_docs.py
git diff --check
```

Use disposable local fixtures for the new browser flows and test databases.
Reuse `backend/test/fixtures/deployment-review` and patterns in
`frontend/test/e2e/post-merge-local.mjs` if compatible; extend isolated fixtures
as needed rather than writing into retained apps. Fake execution is sufficient
for UI/Preview/Deploy lifecycle acceptance here; report it as fake. If persistence
changes, run integration/round-trip/race tests against a newly owned local
PostgreSQL and verify restart; clean only resources created by that test.
Use fake/synthetic Secret values and provider test doubles for new UI tests;
existing VSO/secret-output tests remain required regression coverage. This issue
has no new live cluster/AWS acceptance requirement and does not depend on access
to a user's credentials or existing datasets.

Document preexisting broad-check failures separately with baseline reproduction;
changed-code checks must pass, do not disable assertions or claim full pass.
Deliver browser screenshots, assertions, test commands/results and remaining
limitations in a dated verification record; never raw secrets/state. Agent can
complete the task without asking the user to choose fixture data, implementation
structure or confirm these already accepted product decisions.
