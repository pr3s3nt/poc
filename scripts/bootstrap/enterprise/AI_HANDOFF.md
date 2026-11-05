# Bàn giao cho AI nội bộ: setup bootstrap staging

Đọc `scripts/bootstrap/AGENTS.md` và root `AGENTS.md` trước. Mục tiêu là thay dữ
liệu input để chạy trên target được cấp quyền, không fork code theo công ty.
Không có bằng chứng rằng staging/GitHub Enterprise thật của công ty đã chạy.
Các tests hiện có chỉ chứng minh contract, HTTPS/CA bằng server test và loader
Git với các commit local. Lượt kind trước đó dùng GitHub công cộng, không GHES.

## 1. Thu thập dữ kiện và quyền đã có

Từ tài liệu/hệ thống kết nối của công ty, xác định các dữ kiện sau; hỏi owner nếu
thiếu, không lấy địa chỉ mẫu làm địa chỉ thật:

- Phiên bản Orchestrator/release/tag được duyệt, backend production profile,
  database schema, platform admin/PE và Developer trong cùng Organization.
- GHES HTTPS host/Organization, CA chain, bot, credential file và quyền tạo/ghi
  private app-scoped repositories. API base phải đúng host `/api/v3`.
- Repo/commit chứa portable documents, Terraform modules, template bundle; local
  checkouts đã có exact commits và đúng HTTPS origins. Source repos và CD org của
  preset cùng credential scope; nếu khác org cần quyết định cấu hình riêng.
- Kubeconfig/context/server identity nhìn thấy từ host lẫn backend, Fleet topology
  và target cluster được cấp quyền. Preset cơ bản dùng cùng cluster quản lý/target.
- Vault backend/workload URLs, KV-v2/auth mounts, VSO, quyền tạo role/policy đúng
  app scope; không giả định Vault đã cấu hình Kubernetes auth.
- Registry nội bộ, digest-pinned PostgreSQL/Redis/MySQL/MariaDB/MongoDB images và immutable acceptance images tương thích release và pull access
  của node/namespace. Bootstrap không build/push images hoặc tự lấy registry token.
- URL/route/DNS cho acceptance frontend, TLS trust từ browser/Node, quyền tạo scoped
  Role/RoleBinding nếu `namespaceAccess.enabled` được bật.
- Phạm vi mutation đã được người dùng cấp: target nào, tạo app/resources/CD repos,
  giữ dữ liệu hay cleanup. Không xin lại quyền đã có; không suy quyền staging từ
  file có chữ staging. Không tự chạy production/AWS, sửa shared RBAC hoặc reset DB.

Nếu cần cài Orchestrator/cluster/Fleet/Vault trước, đây là prerequisites có quy
trình riêng. Dùng release/images đã được phê duyệt và runbook vận hành tương ứng;
không chạy script bootstrap để thay installer/migration. Không dùng seed account
hoặc hardcode mật khẩu demo trong môi trường công ty. Migrations theo ADR-027,
owner setup tách runtime; tuyệt đối không reset DB/chỉnh schema marker để qua gate.

## 2. Chuẩn bị dữ liệu staging

1. Giữ thay đổi có sẵn, dùng release có console-only profile. Chuẩn bị nguồn đã
   review từ repo công ty, root có `terraform/`, `templates/`, `bootstrap/`.
2. Copy input mẫu vào thư mục private, điền theo INPUT_REFERENCE. Giữ
   `runtime.profile=console`. Bỏ `runtime.clusterKubeconfigFile`, `gitCacheDir`,
   namespaceAccess và port-forward; platform/RBAC/route là prerequisites.
3. Chọn đúng ba full commits. AI xác nhận snapshot local là bộ nguồn được review;
   helper không dùng Git để chứng minh commit/origin. Không gửi secret lên Git.
4. Chạy `prepare-enterprise-input.mjs` theo README. Helper chỉ đọc files, tính
   pins/hashes, tạo prepared config và `prepared-inputs-*` bên cạnh. Không fetch,
   clone, gọi kubectl hoặc mutate hệ thống. Input/output cùng thư mục để credential
   paths không đổi; file 0600, thư mục 0700, không ghi đè output cũ.
5. Giữ bộ prepared config/documents nguyên vẹn. Snapshot của module/template phải
   khớp Git commit thật: backend inspect qua Console sẽ xác minh content pin.
   Business documents được kiểm SHA local, không có chứng minh remote commit.

## 3. Preflight và chạy qua Playwright

Từ checkout công cụ đã được cung cấp, Node 24 và dependencies/Chromium đã cài:

```sh
node scripts/bootstrap/run.mjs --config /var/lib/orch-bootstrap/staging/staging.prepared.json
```

Bắt đầu mode `check`: kiểm local input/hash và thông tin/capabilities/conflicts
qua Console. Không có kiểm tra node/Fleet/VSO bằng kubectl. Console checks không
chứng minh mọi thành phần cluster hoạt động; trạng thái target được backend xác
minh qua các flow onboarding/verify tương ứng. Thiếu quyền thì dừng bước phụ thuộc
và xử lý platform prerequisites bằng quy trình công ty, không tự cấp RBAC.

Đổi mode `bootstrap` khi được cấp quyền target để cấu hình Types, chín Definitions,
Git module/template sources, account, Vault, Fleet binding/default qua Console UI.
Báo BOOTSTRAPPED riêng với nghiệm thu deploy. Không gọi API mutation để thay UI.
`ui.get` qua browser session chỉ dùng đọc lại trạng thái và đối chiếu receipts.

Mode `all` thêm acceptance app, Preview/Deploy qua Console và browser business/job/
file checks ở URL workload đã có route. App vẫn dùng PostgreSQL/Redis; MySQL,
MariaDB/MongoDB chỉ được đăng ký Definition. Chuẩn bị Secret delivery và registry
pull access riêng theo `projects/platform-resources/bootstrap/DATABASES.md`.

Report Console không có proof đọc Fleet resources/PVC UID hoặc Pod recreation.
Bản video thành công chỉ phản ánh browser checks thực sự chạy. Nếu cần native
proof ở dev, dùng explicit dev-test và các scenario đã review với state/report
khác, không nâng quyền staging bootstrap. Không đổi profile trên state đã có.

Rerun giữ nguyên input/state/identities; khác cấu hình thì BLOCKED, không reset,
đổi tên resource hoặc xóa state để tiếp tục. Đây không phải general upgrade engine.

Frontend mẫu khai báo endpoint; Definition endpoint cấp hostname `<endpointName>.<endpointRootDomain>` qua Gateway đã có.
Điền nhóm tham số httprouteGateway* và endpoint*, kiểm listener/DNS/TLS phù hợp domain gốc và quyền delivery/Fleet apply
HTTPRoute trong namespace ứng dụng. Đọc `projects/platform-resources/bootstrap/HTTPROUTE.md`. HTTPRoute đi cùng workload
manifest, không có Terraform module riêng. Không tạo Gateway/controller hoặc sửa route web khác; không thêm publicRoutes trùng đích này.

## 4. Lỗi và resume

| Tình huống | Xử lý |
|---|---|
| Git 401/403 hoặc scope khác | Kiểm tra quyền/token file/scope; không log token hoặc tự mở rộng quyền |
| TLS error | Sửa CA chain/hostname/trust đúng nơi; không tắt TLS |
| File/tree pin khác | Kiểm tra commit/path/release và review thay đổi; không bỏ digest |
| Existing config khác | Dừng mutation phụ thuộc, so input/server identity; không tự overwrite |
| Outcome UNKNOWN/INTERRUPTED | Xác minh worker đã dừng, dùng recovery cùng persisted proof; không replay/clear lock/state |
| Fleet không Ready | Đọc GitRepo/Bundle/revision, controller status và scoped RBAC; không takeover/force apply |
| Image pull lỗi | Xác nhận image tag/digest và registry access từ target, không đổi sang public image ngầm |
| PVC Pending/mount lỗi | Đọc StorageClass/binding, storage Definition outputs, namespace/mount contract |
| Browser route lỗi | Kiểm tra chính URL sẽ bàn giao; localhost health không đủ |

Giữ reports/state/evidence ngoài Git. Resume cùng state sau khi đã xử lý nguyên
nhân; unknown mutation cần server reconciliation, không bấm lại để thử vận may.
Không xóa app cũ, PVC, remote repo hoặc shared resource khi chưa có cleanup scope.

## 5. Bàn giao và video

Báo cáo: release/commit công cụ, tên target, nguồn repo/full commits/digests,
checks pass/fail/skipped và lý do; IDs do Console cung cấp và nguồn
Definition và Git template; số deployment trước/sau rerun; không ghi PVC UID nếu không quan sát; URL Console/workload
đã mở thật trên browser và cùng-origin API. Nêu rõ từ máy nào đã kiểm tra.

Nếu có yêu cầu video, bật `runtime.recordVideo` đúng phase được nghiệm thu. Chỉ
giữ bản thành công cuối cùng, loại bản lỗi/thử; không quay secret-entry. Video
không thay assertions và không tự chứng minh toàn bộ setup. Mô tả đúng phạm vi clip.
Ghi đường dẫn input/state/report/video, dịch vụ/forward cần giữ, cách chạy lại và
mọi operation còn held. Cleanup phải qua product remove/Preview/Deploy và kiểm
ownership/UID; không dùng lệnh xóa namespace/PVC để làm trạng thái trông sạch.
