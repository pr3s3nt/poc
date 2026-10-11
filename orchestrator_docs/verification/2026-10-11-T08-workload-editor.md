---
id: VERIFY-T08-WORKLOAD-EDITOR-20261011
artifact: verification-record
status: evidence
last_reviewed: 2026-10-11
---

# T08 — Trang Workload editor

## Outcome và phạm vi

WL-01/02, VI-02/04/05 trên branch `refactor_ui`, T01 đã DONE. Working tree
sạch khi bắt đầu; không có user-owned changes. Codex cập nhật UC-16 UI
README/screens/states trước code; Claude implement code/tests qua `clauded`
trong session riêng `codex-T08-20261011`, không sửa docs hoặc commit/push.

Editor/key picker dùng nhãn, aria-label và validation frontend tiếng Việt.
CPU/memory request/limit, resource class và Service target-port override nằm
trong phần nâng cao; tóm tắt giá trị hiệu lực luôn hiện. Request thiếu lấy
limit cùng loại rồi defaults `10m`/`32Mi`; limit không có default. Score không
tự thêm capacity, class rỗng vẫn bỏ field, targetPort fallback vẫn là số.
Imported/edited overrides và multiple aliases giữ nguyên khi collapse hoặc sửa
trường cơ bản; key/Secret mới không tự chọn. Technical IDs, values,
`staging`/`production`, Score và API contract giữ nguyên.

Route chỉ xuất hiện sau thao tác thêm; chỉ tự chọn khi đúng một cổng hợp lệ
(tên không trống/duy nhất, số nguyên 1–65535). Rename/delete/invalid number giữ
đích cũ, hiện lỗi Việt và chặn PUT; không retarget. BUSY/stale/scope guards và
complex Score lossless path giữ nguyên. `publicPort` removal thuộc T25; raw
backend error mapping thuộc T20. Shared shell/Application home labels thuộc
T10/T12, không mở rộng scope.

## Checks độc lập và evidence

- Claude V-FE exit 0: typecheck/lint/test/build, 301 tests/20 files;
  `/tmp/poc-T08-handoff-20261011/logs/fe-checks.log`.
- Codex V-FE độc lập exit 0: typecheck/lint/test/build, 301 tests/20 files;
  `/tmp/poc-T08-handoff-20261011/codex-{typecheck,lint,test,build}.log`.
- `bash -n backend/test/integration/refactor-ui-local.sh` và `node --check`
  bảy e2e modules thay đổi pass. Wrapper không cần đổi cho scenario mới.
- V-UI(T08) exit 0, 20 assertions/22 phase marks, run ID
  `refactor-20261011054102-5176`,
  `/tmp/poc-T08-handoff-20261011/evidence-T08-run2`.
  Giữ raw/captioned MP4, captions.srt/ass, marks.json, assertions.txt,
  result.txt, run.json và frames ngoài Git.
- Codex full decode captioned video exit 0 (`codex-decode.log`), 1440×996,
  386.27 giây; raw 1440×900. Review frames `review-summary.png`,
  `review-invalid.png`, `review-import.png`, `review-advanced.png` dưới handoff:
  browser/cursor rõ, caption Việt dưới browser không che UI, không credential thật.
- T03 regression exit 0, run ID `refactor-20261011054820-3042`,
  `/tmp/poc-T08-handoff-20261011/evidence-T03-regression`;
  `logs/t03-regression.log`. Chứng minh addWorkload helper mới hoạt động, native
  renderer khi không mẫu, selected renderer fail không fallback (test-local stub).
  Codex decode regression exit 0 (`codex-decode-T03.log`), frame/caption reviewed.
- Run1 FAIL vì scenario chưa mở “Xem schema postgres” trước click sao chép;
  đã sửa scenario, giữ toàn bộ fail evidence tại
  `/tmp/poc-T08-handoff-20261011/evidence-T08-run1`, log `logs/t08-run1.log`.
- Review cuối sửa một selector cũ còn sót trong acceptance-kind-human; node syntax
  và stale-label search pass, không rerun live integration. Không sửa product sau
  V-FE/video; một comment ngoài scope được khôi phục.
- Docs checker và whitespace pass trước completion; không đổi PlantUML.

## Adapter boundaries và cleanup

Backend product dùng fake adapters và JSON state disposable. Đăng ký Type
number/bool, tạo ứng dụng/Variables, lưu draft, import hợp lệ/không hợp lệ đi qua
UI/backend local. GET phục vụ assertion: payload gửi khớp draft đã lưu.
Secret `API_TOKEN` trong key catalog là projection mock (chỉ metadata), cùng
một PUT 409 stale và Environment BUSY projection; scenario/caption/assertions
ghi rõ mock. File nhập qua UI file input bằng Playwright `setInputFiles`.
Không Preview/Deploy ở T08; T03 fake Deploy dùng renderer stub như scenario
hiện có. Video không chứng minh adapter thật.

Không đọc `.env` hoặc credentials thật, không nối retained DB/Vault/K8S-4F/AWS,
không upload evidence. Runner cleanup chỉ process/state của run, giữ fail/pass
artifacts. Go/conformance/SQL opt-in/live kind/AWS không chạy vì không sửa backend
product và external verification không thuộc scope. Build Vite có cảnh báo chunk
trên 500 kB, build vẫn pass; không đổi bundling trong task này.

Docs/current state/code map/traceability/refactor status đồng bộ outcome. Không
user-owned changes bị sửa hoặc còn lại. Session task đóng sau review/validation;
commit/push chỉ T08 trên `refactor_ui`.
