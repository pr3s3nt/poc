---
id: VERIFY-T07-ENVIRONMENT-SETTINGS-20261011
artifact: verification-record
status: evidence
last_reviewed: 2026-10-11
---

# T07 — Trang Cài đặt môi trường

## Outcome và phạm vi

ENV-01, VI-02/04/05 và labels CFG-01 trên branch `refactor_ui`; T01 đã DONE.
Working tree sạch khi bắt đầu, không user-owned changes. Codex cập nhật UC-01
và UC-12 UI screens/states trước code. Claude implement qua `clauded` trong
session tmux riêng `codex-T07-20261011`, không sửa docs/commit/push. Giao diện
interactive không xử lý prompt; chế độ nhận brief trực tiếp `clauded -p` trong
cùng session hoạt động.

Hai selector gợi ý draft chưa lưu khi binding trống và chỉ có một READY choice
hợp lệ; nhiều lựa chọn yêu cầu chọn tường minh, không có thì hướng dẫn/retry.
Chỉ Lưu đổi binding với versions hiện hành; không dùng Organization default hoặc
fallback. Target đã cấu hình không bị thay. Reload/stale giữ pending choice và
nội dung editor; reply sai app/env không đi vào cấu hình đang xem. Profile/region
chỉ đọc. Nhãn/dialog/aria-label Settings, configuration, transition và recovery
được Việt hóa; IDs, values, enums và payload không đổi. Đổi tên không rewrite
tham chiếu; downtime và prior-execution-stopped confirmations giữ nguyên.

## Checks và evidence ban đầu

- Claude V-FE ban đầu exit 0: typecheck, lint, test (283 tests/20 files), build.
  Log `/tmp/poc-T07-handoff-20261011/checks-fe.log`.
- V-UI(T07) run3 PASS, 21 assertions/28 marks, run ID
  `refactor-20261011045628-10039`; `/tmp/poc-T07-evidence-1791694587-3`.
- T01 regression PASS; `/tmp/poc-T07-evidence-T01reg-1791695072`.
- Codex full decode video run3 exit 0, log
  `/tmp/poc-T07-handoff-20261011/codex-decode-initial.log`.
  Raw 1440×900, captioned 1440×996, khoảng 422 giây. Review frame gợi ý và
  transition tại `review-initial-single.png`, `review-initial-transition.png`
  dưới thư mục handoff: browser/cursor rõ, caption Việt dưới browser không che UI.

## Adapter boundaries và giới hạn

Backend adapters fake, state/credential store memory không bền vững; không đọc
`.env`, dùng credential thật, nối retained DB/Vault/cluster/cloud hoặc upload.
Create app, single-choice suggestion, Connection Save, Store register/Save,
copy một Secret A→B, variable/secret CRUD và rename/delete unreferenced key qua
UI với backend fake. API GET chỉ phục vụ assertions. Secret input được che,
readback chỉ metadata; snapshot không ghi Secret plaintext.

Playwright route **projection mock** cho nhiều/không Connection choices,
copy failure 502, stale/BUSY 409, usedBy warning và runtime/operation state.
Toàn bộ transition preview/execute/progress/cleanup/recovery là mocked API,
không chứng minh migration/copy trên adapter thật. No-store state qua backend
fake thật. Các live e2e scripts chỉ cập nhật selector/syntax; không chạy kind/AWS.
Thông báo do backend trả về còn theo contract hiện tại; VI-03 mapping tiếng Việt
đầy đủ thuộc T20. Shared shell/Application home entry labels thuộc T10/T12.
Không Go/conformance/SQL opt-in vì không sửa backend product; wrapper build Go
binary local, không cấp quyền external mutation.

Run1 FAIL do assertion không còn đúng khi single-choice đã được gợi ý; run2
FAIL do dùng getAllByRole của Testing Library trên Playwright Locator. Scenario
đã sửa, không nới contract product; giữ video/captions/marks/assertions tại
`/tmp/poc-T07-evidence-1791694291-1` và `/tmp/poc-T07-evidence-1791694447-2`.
Runner chỉ cleanup process/state của run, không xóa evidence/retained resources.

## Review cuối

Codex review yêu cầu sửa helper bỏ qua Lưu khi draft gợi ý cùng key, giữ editor
mới khi Save cũ còn pending, không refresh configuration sai Environment sau
store Save muộn, invalid/lost destination và nhãn Vùng/profile. Claude sửa trong
cùng session; tests bổ sung cho delayed Save/editor/unmount/scope/lost choices.

- Claude V-FE cuối exit 0, 289 tests/20 files;
  `/tmp/poc-T07-handoff-20261011/checks-fe-review.log`.
- Codex V-FE độc lập exit 0: typecheck/lint/test/build, 289 tests/20 files;
  `codex-{typecheck,lint,test,build}.log` dưới thư mục handoff.
- Headed V-UI(T07) cuối exit 0, 21 assertions/28 marks, run ID
  `refactor-20261011051044-6676`, `/tmp/poc-T07-evidence-review-1791695443`.
  Raw/captioned MP4, captions.srt/ass, marks.json, assertions.txt, result.txt,
  run.json giữ ngoài Git. Raw 1440×900/captioned 1440×996, khoảng 424 giây.
- Codex full decode captioned exit 0 (`codex-decode-final.log`); review frames
  gợi ý visible, secret update trống, copy failure, transition/recovery tại
  `review-final-*.png` dưới handoff. Cursor/browser rõ, phụ đề dưới cửa sổ,
  không che controls hoặc lộ credentials thật. Wrapper raw/captioned decode pass.
- T01 regression pass trước review; không rerun vì các trang T01 không đổi.
- `bash -n` wrapper, `node --check` các e2e mjs, docs checker và diff whitespace
  pass. Kind/AWS scripts syntax only; không có live mutation/SQL opt-in.

Canonical UI docs, current state, code map, traceability và refactor status đã
đồng bộ outcome. Không user-owned changes bị sửa hoặc còn lại. Session tmux task
được đóng sau review/validation; chỉ stage/commit/push thay đổi T07.
