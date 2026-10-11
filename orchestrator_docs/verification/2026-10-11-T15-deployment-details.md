---
id: VERIFY-T15-DETAILS-20261011
artifact: verification-record
status: evidence
last_reviewed: 2026-10-11
---

# T15 — Chi tiết triển khai

## Outcome và phạm vi

VI-02/04/05, branch `refactor_ui`; T01/T10 DONE, working tree sạch ban đầu.
Codex đọc kế hoạch và UC-09, cập nhật UI screens/states trước code. Claude
implement qua `clauded -p` trong session `codex-T15-20261011`: giao diện tương
tác không nhận prompt nên chuyển print mode trong cùng session. Claude không
sửa docs/commit/push. Codex review và giao vòng sửa trong cùng session.

Trang detail dùng tiếng Việt cho metadata, trạng thái/hành động, graph/batches,
loading/empty/error/not-found/retry; thời gian vi-VN. IDs, descriptors, keys,
digest, snapshot lịch sử và API giữ nguyên. “Tập triển khai”, “Cấu hình tài
nguyên” và UNREFERENCED theo glossary; không thêm live status/rollback hoặc
lấy đích hiện tại thay dữ liệu lịch sử. API không trả thêm target binding.
Failure có hướng dẫn Việt tổng quát, không render raw reason; mapping chi tiết
code-based thuộc T20. Marker bí mật hiện “Đã che”; nested output ẩn, resolved
inputs không render. Scope/ID/lượt đọc cũ bị loại bỏ khi response/error đến muộn.

## Checks và review

- Claude V-FE cuối exit 0: typecheck/lint/375 tests (20 files)/build. Logs và
  exit status ở `/tmp/poc-T15-evidence-logs/`, các file `*-review.log` và
  `exit-review.log`. Codex độc lập chạy toàn V-FE cuối, exit 0, 375 tests;
  build log `/tmp/poc-T15-handoff-20261011/codex-final-build.log`. Build còn
  cảnh báo chunk >500 kB hiện hữu.
- Codex wrapper `bash -n`, syntax `node --check` bốn mjs thay đổi, docs checker
  và whitespace exit 0. Tests kiểm GET-only retry, scope/ID/attempt late replies,
  late 404/500, labels, graph direction/batch order, historical digest,
  no-plan và safe raw/nested/input suppression.
- Review sửa missing UNREFERENCED và Workload APPLYING/REMOVING/REMOVED, nhãn
  tĩnh, hướng dẫn Xem trước trước retry, partial detail locator migration và
  test late 404 có request thật sự pending. Tên test phản ánh đúng assertion.
- Go/SQL/conformance/PlantUML không chạy vì không sửa backend/schema/diagram.
  Ba legacy e2e files chỉ syntax-check, không chạy môi trường integration của
  chúng trong scope này; một số history locators đã stale từ T14, không mở rộng
  sửa chúng. Không coi syntax-check là integration pass.

## Browser evidence

Primary `/tmp/poc-T15-evidence-20261011-review-01`, result PASS; 10 assertions,
14 marks, 8 projection mock activations, raw/captioned MP4, captions SRT/ASS,
assertions, marks, mocks/run JSON và frames được giữ. Video 204.1 giây; raw
1440×900, captioned 1440×996. Codex full decode cả hai exit 0; review frames
real detail, mock success/redaction/UNREFERENCED, failure, loading và captioned
frame ở 141 giây. UI không trống, cursor rõ, phụ đề Việt dưới UI không che nội
dung, không thấy credentials. Caption bước mock còn thuật ngữ kỹ thuật
Definition/graph/batches; nhãn sản phẩm đã Việt hóa.

Product bước UI: đăng nhập, tạo app, thêm api/web, Preview/Deploy qua backend
local với fake adapters/fake deliverer tạo hai SUCCEEDED Deployments, đọc detail
có plan persisted, giữ ID/digest/actor; 404 thật khi mở ID staging dưới production,
retry GET về backend local. Sau Deploy không có non-GET request và số Deployment
vẫn hai. API reads chỉ phục vụ assertions; không chứng minh provider thật.

GET detail projection mocks kiểm graph/matches/batches, historical digest,
secret marker/nested/resolvedInputs giả, UNREFERENCED; FAILED không plan/raw
reason giả; ba lifecycle states; 404/500 và explicit-gate late reply. Gate chỉ
nhả response api sau khi web đã tải, chờ response finished trước assertion.
Mocks không persist và được khai riêng trong evidence.

Lượt đầu `/tmp/poc-T15-evidence-20261011-01` PASS, giữ nguyên sau review.
Regression T14/T01 trước vòng sửa pass ở
`/tmp/poc-T15-evidence-20261011-reg-T14-01` và
`/tmp/poc-T15-evidence-20261011-reg-T01-01`; không chạy lại vì vòng sửa chỉ trang
detail/scenario T15, không đổi runner/helper/history/sign-in. Không có lượt fail.
Brief/report ở `/tmp/poc-T15-handoff-20261011/` ngoài Git.

## External và handoff

Không đọc .env/credential thật, không mutate DB/Vault/K8S-4F/AWS; giữ retained
resources. Runner cleanup process/state của run, evidence không upload. Không
có user-owned changes ban đầu; mọi thay đổi commit thuộc T15. Codex cập nhật
DONE, commit/push prefix T15 và đóng session task sau validation. Tra SHA bằng
`git log --grep '^T15:'`.
