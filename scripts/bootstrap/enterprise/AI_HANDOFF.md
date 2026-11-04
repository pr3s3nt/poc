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

## 2. Chuẩn bị dữ liệu

1. Xem branch/diff và giữ công việc có sẵn. Dùng đúng release đã có Enterprise
   host/CA fix và source-summary UI fix. Kiểm tra tài liệu code-map/current-state.
2. Đảm bảo root repo internal chứa `terraform/`, `templates/`, `bootstrap/`
   (upload nội dung `projects/platform-resources/`, không thêm tầng thư mục). Nếu chuyển
   nguồn sang path khác, cập nhật input tương ứng; commit
   bằng quy trình publish đã được cấp quyền. Không tự push ngoài scope.
3. Nếu nhận source ZIP thì ZIP không có Git history: đưa nguồn vào repo nội bộ
   theo quyền publish đã có, commit và checkout lại đúng HTTPS origin trước khi lock.
   Dùng npm registry, provider mirror và image registry nội bộ theo chính sách công ty;
   không giả định runner có Internet.
4. Copy `staging.input.json` và hướng dẫn vào thư mục private của target. Đặt owner
   permissions: thư mục 0700, credential/input 0600 hoặc 0400 khi chỉ đọc. Không
   đặt state/report/cache dưới checkout. Không ghi token thật vào tham số hoặc JSON.
5. Điền từng dòng theo `INPUT_REFERENCE.md`. Giữ riêng config đầu vào có thể review;
   điền 3 full commits, không branch/tag/latest. Registry image phải có tag cố định
   hoặc digest. Nếu bỏ custom CA phải thực sự là certificate được hệ thống trust.
6. Lock từ local Git objects bằng lệnh trong README. Tool không đọc working-tree
   edits, không fetch/clone/push. Không ghi đè lock cũ; dùng tên lock mới để review
   sự thay đổi và giữ nguyên vị trí tương đối của credential/state.

Lock tính file SHA-256, tree SHA-256 và Git source ID. Module ID nhận diện theo
content/release giống backend; nếu backend khác release có thể khác trusted
metadata. Khóa hash không chứng minh Terraform input/output/provider contracts
hoặc workload TemplatePin đúng; Console inspection vẫn bắt buộc. Helper cơ bản
chỉ nhận regular nonexecutable files; custom module/child modules/phụ thuộc ngoài
cần được inspect và pin theo đúng policy, không bỏ validation để tiếp tục.

Ba Definitions database bổ sung dùng `terraform/mysql`, `terraform/mariadb`,
`terraform/mongodb`; điền nhóm tham số image/Secret/storage trong INPUT_REFERENCE.
Secret data và pull credentials không do script tạo. Đọc
`projects/platform-resources/bootstrap/DATABASES.md`; app nghiệm thu không tự dùng
ba database này, đăng ký thành công không chứng minh chúng đã chạy trên staging.

## 3. Preflight và chạy

Từ root checkout, Node 24 và dependencies/Chromium đã cài:

```sh
node scripts/bootstrap/run.mjs --config /var/lib/orch-bootstrap/staging/staging.locked.json
```

Lần đầu giữ `runtime.mode=check`. Phải thấy check Git file digests của platform, Console
health/schema/module/template capabilities, cluster/Fleet/VSO prerequisites và
compatibility/config conflict checks. Đây có kết nối thật nhưng không seed/deploy.
Không bật debug/trace chứa credential. Các vấn đề TLS phải sửa trust/hostname/CA,
không dùng insecure flags. Đảm bảo proxy/firewall cho runner, backend, Git writer,
Fleet và image pull; thành công từ laptop không chứng minh container truy cập được.

Khi target được cấp quyền và preflight pass, đổi mode sang `bootstrap` để seed
platform. Kiểm tra Types, chín Definitions, Git source pins, account, Secret Store,
verified Fleet binding và platform default. Nếu default hiện có khác input,
bootstrap dừng; không ghi đè. Chỉ BOOTSTRAPPED chưa phải app nghiệm thu.

Đổi mode `all` để seed và deploy acceptance app riêng. Full run phải có app không
Catalog, Preview/Deploy qua Console, CD/Fleet exact-commit readiness, 3 workloads,
5 business checks, job DONE, backend mounted Bound owned PVC `/data` và file/hash.
Lưu ý URL workload trong preset cần route thực tế tới frontend; script không tự
biến một hostname nhập vào thành DNS/Ingress hoạt động. Cấu hình route/forward
qua cách truy cập được công ty phê duyệt trước phần browser verification.

Chạy lại mode `all` cùng input/state. Phải không có deployment/revision/resource
mới, Git/Definition identities giữ nguyên. Script chỉ reuse hoặc tạo thiếu; input
khác trên target có sẵn sẽ BLOCKED, không phải general upgrade engine. Đừng xóa
state/rename resource để lách conflict. Thay đổi có chủ đích sau bootstrap cần
quy trình review/upgrade riêng của sản phẩm.

Nếu cần chứng minh file qua lần tạo lại Pod ở staging, xin phạm vi riêng nếu chưa
được cấp và dùng scenario đã review cho target đó. Runner `bootstrap-git-pvc`
hiện chủ động chặn ngoài dev. Không đổi tên environment thành dev để vượt guard.

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
checks pass/fail/skipped và lý do; IDs/UID của app/namespace/PVC/deployments, nguồn
Definition và Git template; số deployment trước/sau rerun; URL Console/workload
đã mở thật trên browser và cùng-origin API. Nêu rõ từ máy nào đã kiểm tra.

Nếu có yêu cầu video, bật `runtime.recordVideo` đúng phase được nghiệm thu. Chỉ
giữ bản thành công cuối cùng, loại bản lỗi/thử; không quay secret-entry. Video
không thay assertions và không tự chứng minh toàn bộ setup. Mô tả đúng phạm vi clip.
Ghi đường dẫn input/state/report/video, dịch vụ/forward cần giữ, cách chạy lại và
mọi operation còn held. Cleanup phải qua product remove/Preview/Deploy và kiểm
ownership/UID; không dùng lệnh xóa namespace/PVC để làm trạng thái trông sạch.
