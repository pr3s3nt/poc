---
id: VERIFY-T09-CREATE-APPLICATION-20261011
artifact: verification-record
status: evidence
last_reviewed: 2026-10-11
---

# T09 — Trang Tạo ứng dụng

## Outcome và phạm vi

APP-01, VI-02/04/05 trên branch `refactor_ui`, T01 DONE. Working tree sạch khi
bắt đầu, không user-owned changes. Codex cập nhật UC-01 UI screens/states trước
code; Claude implement qua `clauded -p` trong tmux `codex-T09-20261011` sau khi
TUI không nhận submit. Claude không sửa docs hoặc commit/push.

Form và lỗi dùng tiếng Việt. Slug gợi ý từ tên chuyển dấu tiếng Việt/đ/Đ,
gom separators, bỏ gạch ngang đầu/cuối và giới hạn 63 ký tự. Gợi ý cập nhật
đến lần sửa subdomain thủ công đầu tiên; sau đó giữ lựa chọn, kể cả xóa trống.
Không có slug thì yêu cầu nhập subdomain. Backend vẫn kiểm uniqueness và sinh
ID. Payload chỉ name/subdomain; endpoint staging/production và điều hướng
Application home staging giữ nguyên. Không thêm target/provider.

Duplicate/invalid errors theo status/field có thông báo Việt; lỗi khác dùng
fallback an toàn, không hiện raw backend text. Form khóa khi submitting, giữ
input sau lỗi và chỉ retry tường minh. Mapping toàn backend thuộc T20; shared
shell/list/home localization thuộc T10/T11/T12.

## Validation và evidence

- Claude V-FE pass, log `/tmp/poc-T09-logs/vfe.log`.
- Codex độc lập typecheck/lint/test/build exit 0: 309 tests/20 files; build log
  `/tmp/poc-T09-handoff-20261011/codex-build.log`. Vite cảnh báo chunk >500 kB,
  build vẫn pass; không mở rộng scope bundling.
- V-UI(T09) PASS: `/tmp/poc-T09-evidence-run2`, run ID
  `refactor-20261011092413-9128`, 10 assertions và 11 phase marks.
  Evidence giữ `T09-raw.mp4`, `T09-captioned.mp4`, `captions.srt/ass`,
  `marks.json`, `assertions.txt`, `result.txt`, `run.json` và `frames/`.
- Codex full decode exit 0, captioned 1440×996, 133.60 giây;
  `/tmp/poc-T09-handoff-20261011/codex-decode.log`. Review suggestion,
  custom/invalid/duplicate frames và captioned video: browser/cursor rõ,
  phụ đề Việt dưới browser không che form, dữ liệu synthetic.
- Run1 FAIL do thao tác nhập tiếp trong scenario chèn chữ ở vị trí cursor;
  sửa scenario thành replace tên đầy đủ, không đổi product.
  Giữ evidence `/tmp/poc-T09-evidence-run1-failed` và log
  `/tmp/poc-T09-logs/vui-run1-failed.log`.
- T01 regression PASS: `/tmp/poc-T09-regress-T01`;
  `/tmp/poc-T09-logs/regress-T01.log`; 8 assertions, Codex decode pass.
- T08 regression PASS: `/tmp/poc-T09-regress-T08-run2`;
  `/tmp/poc-T09-logs/regress-T08-run2.log`; shared create helper giữ payload đúng
  khi form có gợi ý. Lượt đầu timeout 280 giây (scenario dài hơn), giữ tại
  `/tmp/poc-T09-regress-T08-timeout`; chạy lại với timeout 590 giây.
  Codex full decode run2 exit 0 (`codex-decode-T08.log` dưới handoff).
- Docs checker, whitespace, shell syntax và node syntax các e2e files đổi được
  kiểm tra trước commit. Không đổi PlantUML.

## Adapter boundaries và cleanup

Backend fake adapters với JSON state disposable. Sign-in, tạo ứng dụng, lỗi trùng
name/subdomain và navigation đi qua UI/backend local. GET chỉ phục vụ assertions.
Một POST bị mock 500 sau khoảng 2 giây để kiểm khóa form và async failure;
caption/assertion đánh dấu MOCK. Thành công và lỗi trùng không mock.

Video không chứng minh real adapters. Không đọc `.env`/credentials thật, không
nối retained DB/Vault/K8S-4F/AWS hoặc upload evidence. Runner chỉ cleanup process
và state của run, giữ fail/pass artifacts. Go/conformance/SQL opt-in/live tests
không chạy vì không sửa backend product và không có external verification trong
scope. Các runner cũ chỉ cập nhật selector và replace subdomain cho gợi ý mới;
không tuyên bố đã chạy lại live integration.

Claude handoff/exit 0 tại `/tmp/poc-T09-handoff-20261011/result.md` và
`claude-exit.txt`; Codex review độc lập diff/checks/evidence. Không user-owned
changes còn lại; session task được đóng sau completion. Commit/push chỉ T09.
