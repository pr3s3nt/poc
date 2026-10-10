---
id: UI-ARCHITECTURE-INDEX
artifact: shared-ui-design
status: current
last_reviewed: 2026-10-10
---

# Shared Web Console UI

Shared UI design owns the application shell and reusable interaction patterns.
It does not own a use-case screen: each use case owns its screens under
`usecase/UC-xx/ui/`.

## Shared shell

- A signed-in page has a persistent left sidebar with `Applications` and
  `Deployments` navigation.
- The footer of the sidebar shows authenticated User name and role, plus a
  sign-out action.
- Main content owns the page title, primary action and use-case-specific state.
- The sign-in page is intentionally outside the authenticated shell.

## Shared interaction rules

- Primary actions are blue; destructive actions require a clear confirmation in
  the owning use case.
- `staging` and `production` are visible Environment tabs, not a generic select.
- Status is expressed through a readable text label and color, never color alone.
- Every screen declares loading, empty, validation, API-error and success states
  in its use-case UI design.

## Use-case UI designs

- [UC-00 sign in](../../usecase/UC-00/ui/README.md)
- [UC-01 application onboarding and home](../../usecase/UC-01/ui/README.md)
- [UC-12 Application Variables & Secrets](../../usecase/UC-12/ui/README.md)
- [UC-16 workload configuration](../../usecase/UC-16/ui/README.md)

## Ngôn ngữ Console và locator (VI-01)

Web Console dùng nhãn tiếng Việt hard-code, không thêm i18n hoặc bộ chọn ngôn
ngữ. Nhãn chuẩn nằm trong [glossary](../../GLOSSARY.md#nhãn-web-console-vi-01).
Text tĩnh, validation frontend, aria-label, trạng thái loading/empty/error,
notification và trợ giúp của trang được Việt hóa cùng task của trang đó.
Các trang chưa chuyển đổi giữ locator hiện tại cho tới task tương ứng; đây là
trình tự triển khai, không phải chế độ tiếng Anh của sản phẩm.

Giữ nguyên tên do người dùng đặt, technical IDs, enums/payload, `staging`,
`production`, image, descriptor, module, key, JSON, placeholder và URL. Không
Việt hóa raw backend error bằng regex; mapping lỗi theo code thuộc task riêng.
Accessible name phải mô tả mục đích control. Browser tests dùng role/label và
nhãn chuẩn theo trang, không thay assertions bằng pause hoặc bỏ kiểm accessibility.

Runner refactor local dùng backend fake và state disposable ngoài repository.
Scenario đi qua UI, có headed browser, cursor/click, pause để đọc và phụ đề Việt
ở dưới video. Giữ video, phase marks và assertions cả khi fail; evidence fake
không chứng minh adapter thật. Quy trình chạy và cleanup nằm trong
[Claude tmux runbook](../../operations/claude-tmux.md).
