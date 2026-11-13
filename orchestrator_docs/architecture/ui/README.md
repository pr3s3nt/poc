---
id: UI-ARCHITECTURE-INDEX
artifact: shared-ui-design
status: current
last_reviewed: 2026-10-28
---

# Shared Web Console UI

Shared UI design owns the application shell and reusable interaction patterns.
It does not own a use-case screen: each use case owns its screens under
`usecase/UC-xx/ui/`.

## Kế thừa giao diện sản phẩm

Áp dụng cho mọi màn hình Web Console, kể cả mock và prototype. UI mới kế thừa
nhận diện, shell, navigation, typography, spacing, màu và interaction patterns
của sản phẩm; chỉ thiết kế phần nội dung thuộc phạm vi task. Không coi yêu cầu
“làm mock” là yêu cầu redesign. Chỉ đổi pattern chung khi người dùng yêu cầu
hoặc shared design đã phê duyệt thay đổi đó.

Các điểm bắt đầu để đối chiếu implementation:

- [AppShell](../../../frontend/src/app/AppShell.tsx): nhận diện, navigation, footer.
- [Styles](../../../frontend/src/styles/index.css),
  [tokens](../../../frontend/src/styles/tokens.css) và
  [app styles](../../../frontend/src/styles/app.css): layout và visual styles.
- [Button](../../../frontend/src/shared/ui/Button.tsx) và các shared components
  cùng thư mục: controls hiện hành.
- [ApplicationsPage](../../../frontend/src/features/applications/ApplicationsPage.tsx):
  ví dụ page header, actions, cards và empty state. Chọn thêm trang tương tự với
  task; không mặc định mọi trang đều dùng layout danh sách ứng dụng.

Ưu tiên import/reuse components và styles trong mock. Nếu môi trường prototype
không cho phép reuse, bám sát reference và ghi rõ phần mô phỏng. Mock cũ chỉ là
reference khi đã được duyệt và còn phù hợp; không thay thế shared design.
Design/code không khớp phải được ghi nhận theo quy trình deviation, không tự
sửa tài liệu cho khớp code. Việc đối chiếu không tự cấp phạm vi sửa mismatch cũ.

## Nội dung UI ngắn gọn, ưu tiên tác vụ

Chỉ hiển thị nội dung giúp người dùng hiểu trạng thái, đưa ra quyết định hoặc
thực hiện hành động. Không biến màn hình thành tài liệu hướng dẫn. Quy tắc này
áp dụng cho toàn Console, không riêng dashboard hoặc một use case.

- Dùng nhãn ngắn và rõ. Bỏ mô tả chỉ nhắc lại nhãn, tiêu đề hoặc điều control
  đã thể hiện; không thêm đoạn giới thiệu mặc định cho mỗi trang/section/field.
- Mặc định chỉ hiện thông tin cần cho tác vụ hiện tại. Định nghĩa, công thức,
  ví dụ dài và giải thích kỹ thuật đưa vào chi tiết, trợ giúp hoặc phần mở rộng.
  Trợ giúp phải dùng được bằng bàn phím và cảm ứng, không chỉ hover.
- Một thông tin chung chỉ trình bày một lần trong phạm vi liên quan. Không lặp
  thông báo, cảnh báo hoặc cùng một trạng thái trong nhiều panel.
- Theo quy tắc ngôn ngữ Console bên dưới; không hiển thị song ngữ mặc định.
  Thuật ngữ gốc có thể nằm trong trợ giúp khi cần đối chiếu.
- Ưu tiên defaults hợp lý, nhóm trường và tương tác rõ để giảm lời giải thích.
  Không cắt chữ bằng cách thay nhãn hành động bằng icon khó hiểu.
- Giữ thông tin thiết yếu tại nơi quyết định: lỗi và cách sửa, hậu quả thao tác,
  phạm vi áp dụng, đơn vị, dữ liệu thiếu hoặc giới hạn làm thay đổi ý nghĩa kết quả.
  Không giấu thông tin này chỉ để giao diện ngắn hơn.

Trước bàn giao, review khả năng hoàn thành tác vụ và nội dung trùng/thừa cùng
với đối chiếu trực quan. Áp dụng ngay cho UI mới và phần đang sửa; không tự động
biến quy tắc thành task redesign toàn bộ các màn hình cũ.

## Shared shell

- A signed-in page has a persistent left sidebar with `Ứng dụng` and
  `Lịch sử triển khai` navigation.
- The footer of the sidebar shows authenticated User name and role, plus a
  sign-out action.
- Main content owns the page title, primary action and use-case-specific state.
- The sign-in page is intentionally outside the authenticated shell.

### Desktop sidebar and Preview space — 2026-10-23

The left navigation has an explicit keyboard-accessible collapse/expand button.
Expanded mode keeps full labels; collapsed mode keeps a narrow icon rail with
named links/tooltips, active route, authenticated identity and sign-out access.
Persist only this display preference locally; unavailable browser storage must
not prevent navigation. Changing width preserves the mounted page, drafts,
selected review node/file and any in-flight operation. Routes and role gates stay
unchanged. Desktop Preview pages use the released horizontal space rather than
remaining constrained by the old page width; the graph observes its container
resize. Do not add a new mobile layout.

Resource illustrations use distinct, coordinated type colors and light tinted
surfaces: cluster blue, namespace violet, PostgreSQL blue, Redis rose, network
amber and workload teal. Type color is independent of execution state: existing
readable status labels, state badges, selected/focus outlines and error treatment
remain authoritative. Node and inspector use the same icon/type palette; icons
are native SVG and always accompanied by the existing resource labels.

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
- [UC-17 platform effectiveness](../../usecase/UC-17/ui/README.md)

## Ngôn ngữ Console và locator (VI-01)

Web Console hỗ trợ tiếng Việt (mặc định, fallback) và tiếng Anh qua message
catalog nạp lúc chạy theo [ADR-020](../decisions/ADR-020-console-message-catalogs.md).
Nhãn chuẩn tiếng Việt nằm trong [glossary](../../GLOSSARY.md#nhãn-web-console-vi-01);
catalog `vi.json` là nguồn hiển thị của các nhãn đó. Text tĩnh, validation
frontend, aria-label, trạng thái loading/empty/error, notification và trợ giúp
dùng key catalog, không viết chuỗi cứng trong component.

Bộ chọn “Ngôn ngữ” nằm ở footer sidebar (ẩn khi sidebar thu gọn) và trang đăng
nhập; chỉ hiện khi manifest có từ hai ngôn ngữ. Đổi ngôn ngữ lưu lựa chọn rồi tải
lại trang. Ngày giờ và số định dạng theo ngôn ngữ đang chọn. Thêm ngôn ngữ chỉ
cần thêm `frontend/public/locales/<code>.json` và một dòng trong `manifest.json`;
không sửa code.

Giữ nguyên tên do người dùng đặt, technical IDs, enums/payload, `staging`,
`production`, image, descriptor, module, key, JSON, placeholder và URL. Không
Việt hóa raw backend error bằng regex; mapping lỗi theo code theo contract T20 bên dưới.
Accessible name phải mô tả mục đích control. Browser tests dùng role/label và
nhãn chuẩn theo trang, không thay assertions bằng pause hoặc bỏ kiểm accessibility.

Runner refactor local dùng backend fake và state disposable ngoài repository.
Scenario đi qua UI, có headed browser, cursor/click, pause để đọc và phụ đề Việt
ở dưới video. Giữ video, phase marks và assertions cả khi fail; evidence fake
không chứng minh adapter thật. Quy trình chạy và cleanup nằm trong
[Claude tmux runbook](../../operations/claude-tmux.md).

## Catalog and Definition configuration — 2026-11-02

Restore Resource Definitions and Rendering Templates navigation/routes with the
existing shell, forms and role gates. Resource Types links to Definition creation.
Catalog remains the source/release browser. Terraform source controls add Catalog
beside Git, ZIP and registered modules; show source, explicit version and compatible
module selectors. Reuse typed variables/mapping/criteria and existing status/error
patterns. See UC-03 CM-01–04; no shell or theme redesign.

## Shared shell tiếng Việt (T10)

Navigation có accessible name “Điều hướng chính”; các link giữ nguyên routes
và gate: “Ứng dụng”, “Loại tài nguyên”, “Kết nối”, “Kho bí mật”. Mục triển khai chưa có link vẫn disabled, nhãn
“Lịch sử triển khai” / “Sắp có”. Không thêm tính năng điều hướng.

Footer giữ username thực; role ADMIN/PLATFORM_ENGINEER/DEVELOPER hiển thị lần
lượt “Quản trị viên” / “Kỹ sư nền tảng” / “Nhà phát triển”, không đổi role ID.
Nút “Đăng xuất” giữ behavior hiện tại. Shared missing-application state dùng
“Không tìm thấy ứng dụng”; loading “Đang tải ứng dụng…”; lỗi tải “Không thể tải
danh sách ứng dụng.” và nút “Thử lại”. T10 không dịch nội dung list/home/preview/
history/details thuộc T11–T15, không đổi parsing unknown routes.

## Trình bày lỗi theo code — T20 / VI-03

API client giữ status/code/field/params từ
[public error contract](../contracts/operation-contracts.md#public-http-error-contract--t20--vi-03)
và chỉ tạo message tiếng Việt an toàn. Mọi trang dùng code cho lỗi known;
unknown/missing/malformed code và lỗi network có fallback Việt theo thao tác.
Không parse hoặc hiển thị raw English/provider/decoder messages. Không nội suy
unknown field/params hoặc correlation không được server kiểm soát vào alert.

Validation/duplicate giữ draft; Secret Store failure xóa token theo UC-04.
BUSY giữ edits và khóa conflicting actions; stale yêu cầu reload/review hoặc
Preview mới. Session expiry vẫn theo UC-00. Retry GET không replay mutation,
và lỗi network của mutation không chứng minh chưa lưu. Loading/error/not-found
và committed-success/reload-failure vẫn tách biệt; scope đổi bỏ late replies.

## Navigation Hiệu quả platform (UC-17)

Sidebar thêm mục “Hiệu quả platform” sau “Cấu hình kho bí mật”, route
`/ui/platform/effectiveness`, cùng gate PLATFORM_ENGINEER/ADMIN như các mục
platform. Developer không thấy mục và không render trang qua deep link. Mục
“Lịch sử triển khai” vẫn disabled; chi tiết bản ghi mở lịch sử theo Environment
hiện có. Trang thuộc [UC-17 screens](../../usecase/UC-17/ui/screens.md).

## Git rendering source reuse — ADR-021

Mẫu dựng ứng dụng reuses the current Terraform Git source inspection/verified
selector patterns under [UC-03 screens](../../usecase/UC-03/ui/screens.md#git-render-bundle-source--adr-021).
Existing shell/routes/criteria stay; shared helper text and full pin details are
progressive disclosures. New source/inputs/error/provenance labels use VI/EN
catalogs. This scoped extension does not redesign the existing catalog.

## Inline source creation — DEVEX-ENV (2026-10-24)

The workload editor uses one environment-variable table per container, owned by
[UC-16 screens](../../usecase/UC-16/ui/screens.md). Reuse shell/card/button/input
styles; no new visual identity or page navigation is required. Source-specific
controls appear within a row; create forms open one accessible dialog while
parent edits remain mounted. Focus returns to the invoking row. Commit boundaries
are explained at the committing action, not repeated in every row. Shared keys
remain UC-12 entities; adding a dependency remains local desired workload state.

## Connections retirement — 2026-10-26

Bỏ navigation và màn hình Connections. Driver Accounts là entry quản lý danh
tính hạ tầng; deep link `/ui/platform/connections` chuyển tới Driver Accounts.
Giữ shell/styles/account forms hiện hành.

## Direct-entry workload variables — ENV-UX (2026-10-27)

UC-16 keeps a Name/Value table with direct entry as the default. Source selection
is an on-demand searchable Dialog, using the existing shared dialog/focus
pattern. Source kind is metadata, not a mandatory column. Reuse the existing
UC-12 create-only forms and app/environment Secret scope; no local variable
reference chaining. See UC-16 BR-26/27 and its screens.


## Resource selection — UC-16 BR-28–30 (2026-10-28)

Reuse dependency controls and Advanced disclosure. New resources default to
shared within the current app/environment; generated IDs stay out of basic
controls. Existing-resource choices show alias, Type and source workload.
Private scope is advanced. No shell/navigation or cross-Application scope change.

## Git configuration — Git access

Nav/page labels (2026-11-05, user decision): “Cấu hình Git” (EN “Git configuration”)
and “Cấu hình kho bí mật” (EN “Secret store configuration”). Routes unchanged.

Sidebar adds “Cấu hình Git” at `/ui/platform/settings`, in the platform
group with PLATFORM_ENGINEER/ADMIN gating. Reuse the existing Secret Stores
form and shell. [UC-03](../../usecase/UC-03/git-access.md) owns fields/states.

## CD provider configuration — 2026-10-29

PE/Admin navigation adds **Cấu hình CD**, `/ui/platform/cd-providers`, next to
Driver Accounts/Secret Stores. Reuse platform list/form/card/status patterns and
VI/EN catalogs. Developer has no authoring navigation or deep-link access. See
[UC-04 CD configuration](../../usecase/UC-04/ui/cd-providers.md).


## Versioned catalog releases — issue #28

PE/Admin navigation adds **Catalog** at `/ui/platform/catalog` within the existing
shell. Application Environment Settings adds a Catalog selection panel and uses
the existing Preview/Deploy surface. [Screen/states](../../usecase/UC-03/ui/catalog-releases.md)
own this feature; no shell identity, navigation interaction or generic form
pattern is replaced.

## Controlled revision upgrades — 2026-11-03

DV-04 extends the existing Definition/render forms using shared Button, details,
status/error and comparison patterns. Versions/Usage are compact disclosures below
the existing catalog; edit remains the current two-tab form. Environment Settings
adds revision selection beside Catalog using existing staging/production tabs.
No navigation/theme redesign. New labels use VI/EN message catalogs; configuration
rollback is not labelled as restoring database contents.


## Simplified CD connections — issue #31

UC-04 CD authoring uses the existing shell/card/form controls and shared Dialog.
Its four connection/topology fields hide technical namespace, identity and
repository defaults. Inline kubeconfig creation reuses the concealed Driver Accounts
control; new Git credentials create reusable Connections. Workspace changes reset
the selected target identity. Default changes retain existing Application pins.
See [CD screens](../../usecase/UC-04/ui/cd-providers.md); no shell/theme redesign.

## Generic Workload and Template Driver — ADR-035

Resource Definitions now owns workload Definitions alongside Terraform and
existing-cluster Definitions. Remove the separate Rendering Templates navigation;
old bookmarks open Resource Definitions with workload selected. Keep the existing
two tabs, Overview, Review JSON, CAS/error states and Version History. Template
sections are Manifest Templates (Inline/Git/ZIP/Catalog), Template Inputs and
Resource References. Existing placement may supply the cluster through namespace.
Inline uses named file editors; schema/default details remain collapsed.
Workload authoring/import uses Generic Workload and keeps existing env/storage/
scheduling controls and shared config/secret pickers. No shell/theme redesign.

### Saved Definition source identity — 2026-11-04

Use the existing module/template card and `pin-panel` definition list for the
saved source summary (UC-03 SP-01–03). Source origin and immutable version stay
visible; creation/replacement controls are progressive disclosure via Change
source. Registration/cache status does not replace origin. Reuse current styles,
buttons and VI/EN message catalogs; no shell/theme change.

## Console source choices — issue #45 (2026-11-07)

The Console hides Workload file import and exposes only Git for new or changed
Definition sources. Saved provenance remains visible. This overrides the earlier
Git/ZIP/Catalog/inline/registered authoring choices in the affected editors only;
backend capabilities and the Catalog browser remain available. Keep the current
shell, shared controls and form layout; do not add explanatory banners.

## Environment rollback — issue #49

Application Home reuses content-panel, Button, Dialog and current status/error
patterns. A scoped Preview Rollback action shows the target Environment and whole
restore/remove workload summary. Confirmation requires the Application key;
initial-empty rollback explicitly warns that owned deployment resources are deleted.
Busy prevents conflicting actions. During execution keep confirmation/dialog
mounted and disable dismissal; outcome and errors are explicit and never echo raw
provider output. Shell/navigation and the normal Preview/Deploy flow are unchanged.

## Catalog navigation — 2026-11-08

Hide Catalog from the shared sidebar. Existing Catalog routes remain read-only
for history; Console offers no new Catalog authoring or Environment selection.
Keep the shared shell and the Resource Type/Definition navigation unchanged.

## Environment import entry — issue #72

Reuse container group, Name/Value table, Button and Dialog. The file-entry action
aligns right and uses the existing green primary Button with white text; the
return action keeps the secondary style. The table is
the default view; Nhập từ file .env opens text editing, with Quay lại bảng to return.
There is no UI tab label. Grammar/help is a
disclosure; errors and independent Secret commit boundary remain visible where
needed. [UC-16 screens](../../usecase/UC-16/ui/screens.md) owns the extension.


## Catalog registration methods — issue #76 acceptance feedback

Resource Type registration and the Definition creation editor use adjacent
Biểu mẫu / Nhập YAML/JSON tabs, inheriting the existing tabs and panel styles.
The second tab contains both upload and paste/edit. Keep both drafts mounted
across switches, use named tablist/tab/tabpanel relations and arrow/Home/End
keyboard navigation; disable method changes during registration/upload. Definition
edit keeps its existing form without a new registration import entry. Format help
remains a disclosure within the import panel; no shell/navigation change.

## User Guide — issue #82

Sidebar thêm **Hướng dẫn sử dụng / User Guide**, `/ui/user-guide`, dùng cho mọi
vai trò đã đăng nhập. Dùng icon sách cùng hệ SVG, active link và collapsed rail
hiện hành. Theo bổ sung ngày 2026-11-13, entry nằm cuối Điều hướng chính, sau cả Lịch sử
triển khai. Xuất bản bài biến môi trường và Sử dụng Workload; mỗi lần đọc một bài,
chọn bài bằng link có URL và mục lục anchor riêng. Giữ các mục platform và role
gates. [UC-16 GUIDE](../../usecase/UC-16/user-guide-specification.md) sở hữu scope.
Trang hướng dẫn là nơi đọc tài liệu theo tác vụ; không mở rộng nội dung trợ giúp
mặc định trên các màn hình thao tác.
