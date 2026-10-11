---
id: VERIFY-T10-SIGN-IN-SHELL-20261011
artifact: verification-record
status: evidence
last_reviewed: 2026-10-11
---

# T10 — Đăng nhập và shared shell

## Outcome và phạm vi

VI-02/04/05 trên branch `refactor_ui`, T01 DONE; working tree sạch lúc bắt đầu,
không user-owned changes. Codex cập nhật UC-00 UI screens/states và shared UI
README trước code. Claude implement bằng `clauded -p` trong tmux riêng
`codex-T10-20261011`, không sửa docs, commit hoặc push.

SignInPage dùng text/validation/accessible labels tiếng Việt. Mật khẩu vẫn bị
che và xóa khỏi state sau submit; username giữ sau lỗi, không hiện raw backend
error. AppShell dịch navigation/footer/sign-out; ADMIN hiển thị Quản trị viên,
PLATFORM_ENGINEER Kỹ sư nền tảng, DEVELOPER Nhà phát triển. Role IDs, username,
routes/gates và auth/session semantics giữ nguyên. Shared App dịch restoring,
expired notice và application-route loading/error/retry/not-found.

Shared login/locator groups signIn/shell và selectors trong runner cũ được cập
nhật. Không chạy runner live. Nội dung ApplicationsPage/home/preview/history/
details còn chờ T11–T15; không thêm i18n, đổi backend/API hoặc unknown-route
parsing. Shared UI components không cần sửa cho scope này.

## Validation và evidence

- Claude V-FE typecheck/lint/test/build exit 0, 320 tests/20 files; logs và
  statuses tại `/tmp/poc-T10-handoff-20261011/vfe-*.log`, `vfe-status.txt`.
- Codex review phát hiện assertion âm cũ dùng Password, yêu cầu Claude đổi sang
  Mật khẩu để giữ coverage có ý nghĩa; title test được chỉnh đúng assertion.
- Codex độc lập chạy `npm run typecheck && npm run lint && npm test && npm run build`: exit 0, 320 tests/20 files; build log `codex-build.log` dưới handoff. Vite cảnh báo chunk >500 kB, build vẫn pass.
- Docs checker, `git diff --check`, `bash -n backend/test/integration/refactor-ui-local.sh` và `node --check` mọi e2e file đổi được kiểm trước commit.
- V-UI(T10) PASS tại `/tmp/poc-T10-20261011-02`, run ID
  `refactor-20261011095144-16903`; 14 assertions và 14 phase marks. Có
  `T10-raw.mp4`, `T10-captioned.mp4`, `captions.srt/ass`, `marks.json`,
  `assertions.txt`, `result.txt`, `run.json`, `frames/`.
- Lượt T10 đầu FAIL do locator exact không khớp label chứa inline error.
  Chỉ sửa scenario, giữ `/tmp/poc-T10-20261011-01` cùng log
  `/tmp/poc-T10-handoff-20261011/vui-T10-01.log`.
- T01 regression PASS tại `/tmp/poc-T10-T01-20261011-01`; log `vui-T01-01.log`
  dưới handoff. Shared login/navigation selectors hoạt động sau Việt hóa.
- Codex full decode T10 và T01 exit 0, logs `codex-decode.log` và
  `codex-decode-T01.log` dưới handoff. T10 captioned 1440×996, 91.07 giây.
  Review frame invalid login, Developer/PE navigation, renderer ADMIN mock,
  expiry và T01: browser/cursor rõ, phụ đề Việt ở strip dưới, không che UI,
  không hiện password. Frame review lưu trong handoff ngoài Git.
- Restore loading và application-route loading/retry covered bằng unit tests;
  không claim các trạng thái đó có browser evidence.

## Adapter boundaries và cleanup

Fake adapters với JSON state disposable local. Invalid/success sign-in,
restore cookie, sign-out, PE/Developer role navigation và missing application
đi qua backend local. ADMIN chỉ mock GET auth/session, dùng cookie PE thật;
không chứng minh backend admission ADMIN. Expiry SETUP thu hồi phiên bằng API,
sau đó click UI nhận 401 thật và hiển thị notice. API GET chỉ dùng assertions.

Không nối retained DB/Vault/K8S-4F/AWS, không đọc .env hoặc credentials thật,
không upload evidence. Wrapper cleanup process/state do run tạo, giữ artifacts
pass/fail; các process có sẵn không bị chạm. Go/conformance/SQL/live checks
không chạy vì không sửa backend product và external verification ngoài scope.
Không đổi PlantUML. Handoff Claude và Codex checks ở
`/tmp/poc-T10-handoff-20261011/`; commit/push chỉ T10 sau review/validation.
