---
id: VERIFY-T02-RESOURCE-FORM-20261010
artifact: verification-record
status: evidence
last_reviewed: 2026-10-10
---

# T02 — Form Cấu hình tài nguyên

## Scope và outcome

Branch `refactor_ui`; RD-01, RD-02 Kubernetes, RD-03, RD-07 form, UI-01/02
form resource và VI-02/04/05. Codex cập nhật UC-03
[UI screens](../usecase/UC-03/ui/screens.md),
[states](../usecase/UC-03/ui/states.md),
[HTTP mapping](../usecase/UC-03/ui/api-mapping.md) trước khi Claude implement
code/test qua tmux riêng. Working tree sạch lúc bắt đầu; không có user-owned
change ngoài task. Backend API, matcher, seed/profile và reference không đổi.

Form/list Việt hóa; resource form chỉ có Terraform/Kubernetes và không hiển
thị workload Definitions. Auto driver/module theo type, Terraform profile khóa
`aws-eks`, Kubernetes `internal-k8s` hoặc profile rỗng Dùng chung có cảnh báo.
Connection list org-scoped lọc READY/kind, gửi key; Terraform vẫn bắt AWS
Connection đến T16; Kubernetes mặc định Connection rỗng và override ở nâng cao.
JSON/criteria/per-driver choices giữ khi đổi controls/thu gọn/submit lỗi.
ID/JSON validation local, safe lỗi Việt theo status, duplicate 409 và reload
sau committed POST không replay registration. Renderer UI riêng còn chờ T03.

Code chính: `ResourceDefinitionsPage.tsx/.test.tsx`,
`RegistrationStates.test.tsx`, formatter tùy chọn của `useCatalogList.ts`,
CSS warning/hidden rule, T02 scenario và page locators. Runner shell không đổi.
`uc02-04-video-local.mjs` cập nhật selectors/expectations cho trang đã Việt hóa.

## Checks

- V-FE cuối exit 0: typecheck, lint, 18 test files / **169 tests**, build.
  Log `/tmp/poc-T02-coordination-20261010/v-fe.log`, exit record `v-fe.exit`.
  Full suite chạy lại sau hai review fixes cuối.
- V-UI(T02) cuối exit 0, result PASS, **9 assertions / 10 phase marks**.
  Command: `bash backend/test/integration/refactor-ui-local.sh --scenario T02 --headed --captions vi --evidence /tmp/poc-refactor-T02-final-20261010181542`.
  Log `/tmp/poc-T02-coordination-20261010/v-ui.log`, `v-ui.exit`.
- V-UI(T01) regression locator sau Việt hóa exit 0 tại
  `/tmp/poc-refactor-T01-after-T02-20261010180722`; log `v-ui-T01.log` và
  `v-ui-T01.exit` trong coordination directory. Chạy trước review fix CSS;
  fix cuối được kiểm trực tiếp trong T02, không chạy lại T01 vô ích.
- Codex kiểm độc lập `bash -n backend/test/integration/refactor-ui-local.sh`,
  documentation checker và `git diff --check`: pass.
- Search selectors cũ trong frontend: renderer live scenario còn chờ T03,
  các selectors resource trong `uc02-04-video-local.mjs` đã cập nhật.

## Browser evidence và review

Run ID `refactor-20261010111543-25290`, adapters **fake**, temporary JSON state
ngoài repo. Dùng seeded synthetic test account/catalog/Connections; không API
seed thay thao tác product. API GET chỉ assertion. UI: sign-in → resource list →
VPC controls/AWS Connection → Kubernetes shared warning/advanced override →
collapse/show giữ dữ liệu → submit variable sai kiểu → duplicate → sửa ID và
đăng ký VPC → đăng ký namespace profile/Connection rỗng → sign-out.

Evidence cuối: `/tmp/poc-refactor-T02-final-20261010181542/`:

- `T02-raw.mp4`: H.264, 1440×900, 15 fps, 116.07 giây.
- `T02-captioned.mp4`: 1440×996, phụ đề Việt trong strip dưới browser.
- `captions.srt/.ass`, `marks.json`, `assertions.txt`, `result.txt`, `run.json`,
  `ffprobe.json`, `frames/`, decode logs.

Codex full-decode raw/captioned cuối exit 0; review frame cảnh báo shared,
ẩn/mở nâng cao, list sau đăng ký; phụ đề khớp thao tác, cursor hiện, không che
UI hoặc lộ credential thật. Frame review thêm ở
`/tmp/poc-T02-coordination-20261010/final-shared.png` và `final-advanced.png`.

Video PASS trước `/tmp/poc-refactor-T02-20261010180420` không được coi là final:
Codex phát hiện `.editor-grid` ghi đè native `hidden`, khiến advanced vẫn hiện.
Claude thêm scoped hidden CSS và Playwright visibility/toggle assertions rồi
chạy lại. Lookup supported type cũng được sửa thành own-property để custom ID
`constructor` không nhận nhầm prototype; regression test pass. Failed runs và
intermediate evidence giữ nguyên ngoài Git; report đầu có count/path cũ được
supersede bởi exit logs/artifacts cuối và record này.

## Cleanup và giới hạn

State JSON tạm đã xóa; backend/Xvfb/browser/recorder của run đã dừng. Session
Claude task đóng trong handoff, các session người dùng giữ nguyên. Evidence
không commit/upload. Không mutation trên retained K8S-4F, DB, Vault hoặc AWS.

Không V-BE/conformance/PostgreSQL opt-in vì không đổi Go product/planner;
runner chỉ build backend cho fake UI. Không chạy live
`template-engine-kind-human.mjs`: còn nhắm form renderer đã bỏ, phải đổi route/
locator ở T03. Runner `uc02-04-video-local.mjs` chỉ cập nhật labels, chưa chạy
lại; T02 runner hiện hành và toàn frontend checks đã pass. Exact matching
preview/T19, code-based errors/T20, AWS Connection inheritance/T16 chưa thuộc
outcome này. Fake video không chứng minh adapter/provisioning thật.
