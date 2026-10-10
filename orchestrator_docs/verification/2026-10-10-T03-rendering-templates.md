---
id: VERIFY-T03-RENDERING-TEMPLATES-20261010
artifact: verification-record
status: evidence
last_reviewed: 2026-10-10
---

# T03 — Trang Mẫu dựng ứng dụng

## Outcome và scope

UI-02 phần renderer, RD-08 UI ban đầu, VI-02/04/05 trên branch `refactor_ui`.
T01/T02/T02B đã DONE; working tree sạch lúc bắt đầu, không có user-owned changes.
Codex chốt UC-03 UI screens/states/api-mapping và shared UI navigation trước khi
Claude implement bằng `clauded` trong tmux `codex-T03-20261010`.

Trang `/ui/platform/rendering-templates` nằm ngang hàng catalog, chỉ PE/Admin;
Developer không thấy link/form qua deep link. List workload-only, form ID kỹ
thuật/bundle ID và editor criteria T02B. POST cùng API hiện hành gửi
workload/score-k8s/internal-k8s, variables chỉ render_bundle, không Connection,
provision hoặc source. ID/bundle validation, safe Việt errors, duplicate,
freeze khi saving và committed-reload failure/no POST replay được kiểm tra.
Installed-bundle selector còn chờ T18, không có binding/selection semantics mới.

Code/test chính: RenderingTemplatesPage và tests, routes/App/AppShell và tests,
ResourceDefinitionsPage trợ giúp, local scenario/locators, wrapper và CLI stub.
Runner template-engine-kind-human cập nhật route/locator bị ảnh hưởng; không
chạy live. Backend product/matcher/API/specification/ADR-010 không đổi.

## Checks

- V-FE exit 0: typecheck/lint/208 tests (20 files)/build; logs tại
  `/tmp/poc-T03-logs/{typecheck,lint,test,build}.log`, statuses tại
  `/tmp/poc-T03-logs/exit-status.txt`.
- V-UI(T03) cuối exit 0, 12 assertions / 12 phase marks:
  `bash backend/test/integration/refactor-ui-local.sh --scenario T03 --headed --captions vi --evidence /tmp/poc-refactor-T03-20261010220616-2298126`.
- Codex độc lập kiểm wrapper/stub shell syntax, hai runner JavaScript syntax,
  documentation checker và whitespace; pass.
- Go suite/SQL/kind/AWS không chạy: không thay backend product; không external
  mutation/SQL opt-in. Local wrapper có build Go binary vào evidence directory.

## Browser evidence và giới hạn

Run `refactor-20261010150617-1541`, adapters **fake**, temporary JSON state;
score-k8s **test-local CLI stub** cố ý `generate` exit 1, không CLI thật.
Product setup và mutations đều qua UI: Developer tạo hai apps/workload;
PE mở trang mẫu, thử unavailable bundle, đăng ký tpl-web scoped app/staging,
kiểm duplicate; Developer Preview/Deploy hai apps và Preview lại sau failure.
API GET chỉ dùng assertions. Không đọc .env hoặc kết nối retained DB/cluster.

Preview không có mẫu khớp chọn native; native Deploy thành công với fake
delivery và không gọi score-k8s. Preview mẫu khớp chọn tpl-web, không gọi CLI;
Deploy gọi init/generate stub, báo failure và không commit current set;
Preview lại workload vẫn pending, vẫn chọn tpl-web. Đây là no-fallback proof
qua product backend path, không phải giả response browser; không chứng minh
score-k8s output hợp lệ hoặc deployment thật.

Evidence `/tmp/poc-refactor-T03-20261010220616-2298126/` giữ
`T03-raw.mp4` (1440×900) và `T03-captioned.mp4` (1440×996), 261.13 giây,
SRT/ASS, marks.json, assertions.txt, result.txt (PASS), run.json, ffprobe.json,
frames và `score-k8s-stub/score-k8s-calls.log`. Codex full decode hai video
exit 0; review frames trang mẫu/bundle error/native và selected Preview/render
failure; captions nằm trong strip dưới browser, cursor rõ, không che controls
hoặc lộ credential thật. Captioned review frames ở
`/tmp/poc-T03-coordination/{bundle-error,selected-preview,render-fail}-caption.png`.

Lượt pass trước chuẩn hóa log stub được giữ tại
`/tmp/poc-refactor-T03-20261010220109-2273793/`, run
`refactor-20261010150110-20176`; final rerun xác nhận định dạng log chuẩn.
Không publish/upload evidence. Cleanup chỉ process/state của runner.

## Regression và sự cố runner

T02 regression PASS/exit 0 tại
`/tmp/poc-refactor-T02-reg-20261010221117-2321930/`; T02B regression PASS/exit 0
ở `/tmp/poc-refactor-T02B-reg-20261010221336-2321930/`. Status/logs được Codex
đối chiếu độc lập tại `/tmp/poc-T03-logs/reg-T02.log`, `reg-T02B.log` và
exit-status.txt. Codex full decode hai captioned regression video exit 0. Wrapper/locator mở
rộng không làm hỏng các scenario này.

Lượt fail đầu giữ tại `/tmp/poc-refactor-T03-20261010220032-2270922/`:
backend không khởi động vì shebang `/usr/bin/env bash` của stub không resolve
được bash trong PATH bị renderer cô lập. Đổi stub sang `/bin/sh`, chỉ dùng
builtins. Không có browser/video ở run này vì fail trước recording; có
orchestrator.log. Dòng `exit=1` chưa gắn nhãn trong exit-status.txt thuộc run
này; không phải final validation failure. Lượt pass kế tiếp dùng tab literal;
final stub dùng tab trong printf format để định dạng rõ ràng hơn.

Không claim số apply calls từ video: fake deliverer không có projection API
cho calls; UI failure/pending và CLI log chứng minh path không hoàn tất bằng
native fallback. Existing Go rendering-failure tests không rerun trong FE task.
Session task được đóng sau review/validation; không đóng session có sẵn.
