---
id: VERIFY-T06-CONNECTIONS-20261011
artifact: verification-record
status: evidence
last_reviewed: 2026-10-11
---

# T06 — Trang Kết nối

## Outcome và phạm vi

CN-01/02, UI-01, VI-02/04/05 trên branch `refactor_ui`; T01 đã DONE.
Working tree sạch khi bắt đầu, không user-owned changes. Codex cập nhật UC-04
UI screens/states/api-mapping trước code; Claude implement qua `clauded` trong
tmux task riêng `codex-T06-20261011`, không sửa docs/commit/push.

Trang Kết nối dùng tiếng Việt, upload tự inspect sau file-read, paste debounce
500 ms; inspect lỗi có retry tường minh, không tự retry vòng lặp. Một context
tự chọn, nhiều context không đoán theo current-context. Tên gợi ý từ cluster
(fallback context) chỉ khi người dùng chưa sửa; custom/blank giữ nguyên khi
đổi file/context hoặc nhận phản hồi. Đổi document/source vô hiệu summary,
context và file-read/inspect pending. Unmount hủy timer và vô hiệu inspect.
Inspect không lưu; POST đăng ký chỉ sau “Kiểm tra và lưu”, payload giữ
`{name,kubeconfig,context}`. Success xóa form/credential; unknown network
outcome không tự resubmit. Lỗi Việt cố định không hiển thị raw provider text.

Code/test: ConnectionsPage, RegistrationStates tests, locators và scenario T06;
runner dùng memory credential store/no state file như T05. Sáu runner cũ đổi
selectors và bỏ manual inspect đã được thay bằng auto-inspect; chỉ syntax
checked, không chạy các live scenarios. Backend product/API, HOST_CONTEXT và
shell không đổi; T20/T22/T10 còn chờ task tương ứng.

## Checks

- Codex V-FE độc lập exit 0: typecheck, lint, test (270 tests/20 files), build.
  Logs `/tmp/poc-T06-handoff-20261011/codex-{typecheck,lint,test,build}.log`.
- Claude V-FE exit 0, logs dưới `/tmp/poc-T06-handoff-20261011/logs/`.
- V-UI(T06) run3 PASS, run ID `refactor-20261011041628-18229`, 10 assertion
  records/12 phase marks. Evidence `/tmp/poc-T06-evidence-20261011-run3`.
- T01 locator regression PASS, `/tmp/poc-T06-evidence-20261011-T01-regression`,
  log `logs/vui-T01.log` (exit 0).
- `bash -n backend/test/integration/refactor-ui-local.sh`, changed e2e node
  syntax, documentation checker và `git diff --check` pass.
- Không Go suite/conformance/SQL/kind/AWS: không sửa backend product. Runner
  build local Go binary; SQL opt-in/external mutation không chạy.

## Evidence và giới hạn

Backend adapters **fake**, memory credential store và state disposable. Không
đọc `.env`, token/kubeconfig thật hoặc retained DB/cluster/cloud. Browser product
steps sign-in/upload/paste/context/name/check-save/retry/sign-out qua UI;
API GET chỉ assertion. Inspect/parser (200/400) qua backend thật local.
Connection verifier vẫn gọi kubectl; kubeconfig tổng hợp dùng endpoint `.invalid`
bị từ chối 422, không xác minh cluster thật. Playwright route giữ request
inspect rồi thả muộn để kiểm stale response; kết quả parse vẫn từ backend.
Success 201 và list row sau đó là **HTTP projection mock**, không chứng minh
credential persistence, verifier thành công hoặc đăng ký cluster thật.
File-read race, unmount và unknown network outcome kiểm bằng unit tests.

Video run3 raw 1440×900/captioned 1440×996 khoảng 137 giây; giữ captions.srt/ass,
marks.json, assertions.txt, result.txt, run.json và frames. Codex full decode
captioned exit 0 (`codex-decode.log`); wrapper full decode raw/captioned pass.
Review frame name/context, masked credential, verification failure và cleared
success tại `/tmp/poc-T06-handoff-20261011/review-*.png`: browser/cursor rõ,
phụ đề Việt dưới browser không che controls, không lộ credential thật.

Run1 FAIL do human helper pause trước assertion debounce; scenario sửa thành
kiểm trong lúc gõ rồi chờ response. Run2 FAIL do accessible name của textarea
lẫn nội dung kubeconfig trong wrapping label; thêm aria-label tường minh để
tên accessible ổn định và không chứa credential. Không nới assertion để pass;
run1/run2 videos/captions/marks/assertions giữ nguyên tại hai thư mục cùng
prefix, suffix `run1`/`run2`. Không upload evidence. Wrapper cleanup chỉ process
và state của run; retained resources giữ nguyên. Tmux task được đóng sau handoff.
