---
id: VERIFY-T04-RESOURCE-TYPES-20261011
artifact: verification-record
status: evidence
last_reviewed: 2026-10-11
---

# T04 — Trang Loại tài nguyên

## Outcome và scope

RT-01..04, UI-01, VI-02/04/05 trên branch `refactor_ui`; T01/T02 đã DONE.
Working tree sạch lúc bắt đầu, không có user-owned changes. Codex cập nhật
UC-02 và UC-03 UI screens/states/api-mapping trước khi Claude implement qua
`clauded` trong session riêng `codex-T04-20261011`.

Trang dùng tiếng Việt, xem toàn bộ schema GET và copy vào draft độc lập với
ID rỗng; giữ type/required/secret, không suy flags từ tên. Duplicate giữ draft
và không ghi đè nguồn. Whitelist dùng chung với form Definition chỉ có vpc,
k8s-cluster, k8s-namespace, postgres; custom postgres-ha chỉ có contract.
CTA điều hướng query resourceType và prefill sau khi xác nhận catalog/whitelist;
không POST, không ghi đè lựa chọn khi reload. workload dẫn sang Mẫu dựng ứng dụng.
Backend/API/executor/matcher/fixtures không đổi. Metadata authoritative chờ T31,
error code mapping đầy đủ chờ T20. Shared shell chưa Việt hóa đến T10.

Code/test: ResourceTypesPage, resourceSupport, ResourceDefinitionsPage prefill,
routes/App wiring và tests, RegistrationStates, CSS chi tiết/schema; scenario
T04 và locators; runner UC-02 cũ cập nhật selectors của trang đã Việt hóa.

## Checks

- V-FE exit 0: typecheck/lint/tests/build. Logs và statuses tại
  `/tmp/poc-T04-handoff-20261011/logs/`: `vfe-npm_run_typecheck.log`,
  `vfe-npm_run_lint.log`, `vfe-npm_test.log`, `vfe-npm_run_build.log`,
  `exit-codes.txt`. Codex chạy suite cuối độc lập: exit 0, 246 tests/20 files,
  `codex-final-tests.log`.
- V-UI(T04) PASS, 9 assertion records/11 phase marks, final run
  `refactor-20261011030930-9880`, evidence `/tmp/poc-refactor-T04-100929/`.
- V-UI(T02) regression PASS tại `/tmp/poc-refactor-T04-reg-T02-101224/`;
  V-UI(T03) regression PASS tại `/tmp/poc-refactor-T04-reg-T03-101456/`.
  Cả hai exit 0; Codex decode toàn bộ captioned regression video exit 0.
  Logs ở `vui-reg-T02.log`, `vui-reg-T03.log`; assertions giữ native/selected
  renderer semantics và no-fallback sau CLI stub failure.
- Codex kiểm shell syntax wrapper, JavaScript syntax runner UC-02 cũ,
  documentation checker và whitespace: pass.
- Go suite/SQL/kind/AWS không chạy vì không đổi backend product, không external
  mutation/SQL opt-in. Local runner build Go binary ngoài repo. Runner UC-02 cũ
  chỉ syntax checked, không tuyên bố scenario cũ đã chạy lại.

## Evidence và review

Adapters **fake**, temporary JSON state, không đọc .env/retained DB/cluster/cloud.
Seed product catalog là setup; custom postgres-ha đăng ký qua UI, không API seed.
Luồng: sign-in → catalog/details → copy/edit → duplicate error → ID mới → custom
no-CTA → supported CTA prefill → invalid query → workload templates → sign-out.
API GET chỉ kiểm assertions. T03 regression có CLI stub local, không renderer thật.

Final T04 giữ `T04-raw.mp4` (1440×900), `T04-captioned.mp4` (1440×996),
99.2 giây; SRT/ASS, marks.json, assertions.txt, result.txt (PASS), run.json,
ffprobe.json và frames. Codex decode toàn bộ raw/captioned exit 0, review frame
schema/duplicate/prefill: UI/cursor rõ, captions ở strip dưới browser, không che
controls hoặc lộ credential thật. Review frames tại
`/tmp/poc-T04-handoff-20261011/`: `details-caption.png`, `duplicate54.png`,
`prefill-caption.png`; mọi video/evidence ngoài Git, không upload.

Lượt fail đầu `/tmp/poc-refactor-T04-100746/` giữ nguyên video/marks/assertions:
selector option postgres khớp nhiều control; sửa locator scope theo type selector,
không sửa product để hợp thức hóa lỗi. Lượt sau PASS. Cleanup chỉ process/state
của từng runner; retained resources không bị chạm. Codex đóng tmux task sau
review/validation và commit/push, giữ các session có sẵn của người dùng.
