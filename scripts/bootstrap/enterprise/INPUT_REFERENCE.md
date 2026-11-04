# Chú thích input staging

File chạy là JSON chuẩn nên không có comment nội tuyến. Dùng tài liệu này cùng
`staging.input.json`; token/mật khẩu nằm trong file riêng, JSON chỉ chứa đường dẫn.
Không copy địa chỉ/IP/credential hoặc commit demo của cá nhân sang công ty.

| Trường | Cần điền / ý nghĩa |
|---|---|
| `environment` | Danh tính target/state, ví dụ `company-staging`; đổi target phải dùng state khác |
| `parameters.definitionPrefix`, `definitionName` | Prefix ID và tên hiển thị cho mười Definitions; chọn một lần rồi giữ nguyên |
| `parameters.httprouteGatewayName`, `httprouteGatewayNamespace`, `httprouteGatewaySectionName` | Gateway/listener công ty có sẵn; section rỗng để bỏ sectionName, không tạo Gateway |
| `parameters.httprouteHostname`, `httproutePath` | Hostname và PathPrefix web mới; frontend mẫu trỏ Service frontend:8080; verification.url phải dùng đúng scheme listener |
| `parameters.targetClass` | Class cluster/workload và resource references tương ứng |
| `parameters.environmentType` | Loại Environment để matching; preset là `staging` |
| `parameters.postgresImage`, `redisImage` | Image PostgreSQL/Redis từ registry công ty, dạng `registry:port/path@sha256:<64 hex>`; không dùng tag trần/latest |
| `parameters.mysqlImage`, `mariadbImage`, `mongodbImage` | Image registry công ty, bắt buộc `@sha256:<64 hex>`; có hỗ trợ hostname kèm port |
| `parameters.<db>Database`, `<db>Username` | Database và application user mặc định; `<db>` là mysql/mariadb/mongodb |
| `parameters.<db>AuthSecretName`, `<db>RootPasswordKey`, `<db>UserPasswordKey` | Tên Secret có sẵn trong namespace ứng dụng và hai tên key; không điền mật khẩu thật |
| `parameters.<db>Storage`, `<db>StorageClass` | Dung lượng PVC và StorageClass; rỗng chọn class mặc định cluster |
| `parameters.<db>ImagePullSecretName` | Secret pull registry có sẵn trong namespace; rỗng dùng credential node/ServiceAccount |
| `parameters.mongodbRootUsername` | Tài khoản quản trị MongoDB trong auth database `admin`; app user riêng nằm trong database ứng dụng |
| `parameters.backendStorage` | Dung lượng PVC backend, ví dụ `1Gi`; thay đổi sau deploy là thay đổi cấu hình, không tự upgrade |
| `parameters.inputsRepository`, `inputsCommit` | HTTPS repo + full commit chứa `bootstrap/` ở root repo tài nguyên |
| `parameters.modulesRepository`, `modulesCommit` | HTTPS repo + full commit chứa Terraform modules |
| `parameters.templatesRepository`, `templatesCommit` | HTTPS repo + full commit chứa workload templates |
| `parameters.*ModulePath` | Đường dẫn module trong repo; namespace/PostgreSQL/Redis/MySQL/MariaDB/MongoDB/persistent-storage |
| `parameters.moduleSourceId`, `*ModuleId`, `*ModuleSHA256`, `templateTreeSHA256` | Prepare helper tính từ snapshot local (hoặc locker từ Git objects); không đoán/copy ID từ repo demo. Contract/packaged recognition phụ thuộc đúng release đang chạy |
| `console.url`, `username`, `passwordFile` | Origin Console và account Admin/PE đã có; mật khẩu là file owner-only |
| `developer` | Account Developer thật, cùng Console/Organization; bootstrap không tự tạo user |
| `platform.gitAccess.url` | Scope HTTPS của Organization GHES, ví dụ `https://github.company.example/platform` |
| `platform.gitAccess.username`, `tokenFile` | Bot và đường dẫn token; quyền source + CD repository provisioning |
| `platform.gitAccess.caPemFile` | PEM CA cho Git/GHES; nếu dùng public-trust cert thì bỏ trường này, không để file rỗng |
| `platform.gitAccess.repositoryUrl`, `ref` | Kiểm tra connectivity bằng repo inputs và exact commit đã khai báo |
| `platform.account` | Tên account, context, server endpoint và kubeconfig. Endpoint phải tới được từ backend, không dùng loopback của laptop |
| `platform.secretStore` | Tên Vault, URL backend/workload nhìn thấy, KV-v2 mount, Kubernetes auth mount, tokenFile, CA nếu cần |
| `platform.delivery` | Tên Fleet binding, platform Git và default. `config` để discovery tự điền hoặc đặt đúng `workspace`/`targetCluster` nếu có nhiều lựa chọn |
| `platform.resourceTypes`, `definitions`, `namespaceAccess.rulesFile` | Git document refs; prepare helper điền SHA-256 và `git.preparedFile` cho local documents; module/template vẫn Git |
| `platform.templateSources` | Git path của template bundle, commit và tree digest |
| `application.name`, `subdomain` | App nghiệm thu riêng, tránh đụng app đang có |
| `application.environment` | `staging` hoặc `production` của mô hình sản phẩm; không đồng nghĩa cấp quyền target |
| `application.images` | Ba images từ registry nội bộ, immutable tag hoặc digest; không `latest` |
| `application.variables`, `secretFiles` | Dữ liệu nghiệm thu và file secret; giữ đúng hợp đồng acceptance workload |
| `verification.url` | URL thật trỏ frontend acceptance app sau deploy, nhìn thấy từ host Playwright |
| `verification.forward` | Chỉ dev-test. Console profile yêu cầu URL workload đã có route, từ chối port-forward |
| `verification.storage.value` | Chuỗi kiểm thử công khai ghi vào PVC; không dùng dữ liệu thật/bí mật |
| `runtime.mode` | `check` → `bootstrap` hoặc `all`; mode nằm trong file, không qua override ngầm |
| `runtime.stateDir`, `reportDir` | Riêng từng target/profile, ngoài Git; giữ để resume/lũy đẳng. `gitCacheDir` chỉ dùng dev-test |
| `runtime.profile` | `console` cho staging; `dev-test` opt-in CLI/native checks. Schema 3 mặc định console |
| `runtime.clusterKubeconfigFile` | Chỉ dùng trong dev-test; không nhập trong console profile |
| `runtime.recordVideo` | Mặc định false ở staging; nếu bật, chỉ các checks thành công mới lưu video workload; không quay secret-entry |

Tất cả đường dẫn tương đối được tính từ vị trí file input; `~/` chỉ dùng với
đường dẫn file/directory được loader hỗ trợ. Khóa file cùng thư mục để không đổi
nghĩa các đường dẫn. `LOCK_WITH_HELPER` và `REPLACE_*` phải được thay trước khi chạy.
Với custom modules/template catalogs, kiểm tra input/output schema và workload
TemplatePin riêng; thay repository/commit không tự làm contract tương thích.

Registry DNS/CA và pull credential phải dùng được từ node Kubernetes. Bộ này
không tự tạo registry Secret: cấu hình node credential provider hoặc ServiceAccount
imagePullSecrets theo quy trình công ty. Đổi URL image không tự cấp quyền pull.
PostgreSQL image cần tương thích PostgreSQL Alpine UID/GID 70; Redis giữ contract
entrypoint hiện có. Mirror nguyên image đã duyệt hoặc review module nếu đổi distro.

## MySQL, MariaDB và MongoDB

Input đăng ký thêm ba Definitions `class=in-cluster`; app nghiệm thu vẫn dùng
PostgreSQL/Redis. Workload chọn một Type bằng resource declaration, các params
bắt buộc là `image`, `database`, `username`, `auth_secret_name`,
`root_password_key`, `user_password_key`; `storage` tùy chọn. Các params ưu tiên
hơn fallback trong Definition. Khi khai báo workload hãy dùng image đã review
khớp với tham số `<db>Image`. Xem ví dụ ở
[database resources](../../../projects/platform-resources/bootstrap/DATABASES.md).

Chuẩn bị Secret có hai key mật khẩu root/app qua Vault/VSO hoặc quy trình đã
được duyệt, trong đúng namespace trước khi Pod cần chạy. Module không đọc data
Secret, không tạo credential và không xuất password. Consumer nhận password
qua secret delivery của công ty, không qua Type params/output. MongoDB app user
có role readWrite trong database ứng dụng, authSource là database đó; root ở admin.

Các module là single-instance có PVC, chưa có HA/replica set/backup automation.
Đổi biến init trên PVC đã có dữ liệu không tự rotate mật khẩu hoặc migrate DB.
Không xóa PVC/state để khắc phục lỗi credentials. Chỉ dùng image tương thích
entrypoint Docker Official Images của từng engine; MongoDB cần mongosh. Không
đổi sang distribution khác mà chỉ sửa URL nếu entrypoint/UID/đường dẫn khác.

## Quyền của staging runner

Console profile chỉ dùng Playwright và local prepared documents. Không cần Git,
kubectl hoặc gh CLI; không dùng `runtime.clusterKubeconfigFile` và không cho
`platform.delivery.namespaceAccess.enabled=true`. Kubeconfig/token của platform
account vẫn có thể là input cho form onboarding, backend giữ trách nhiệm truy
cập target. Dữ liệu `git.preparedFile` được đọc tương đối từ prepared config,
kiểm SHA-256 và ghi receipt local; không coi đó là bằng chứng remote Git commit.

HTTPRoute: xem [hướng dẫn Gateway có sẵn](../../../projects/platform-resources/bootstrap/HTTPROUTE.md).
Cần HTTPRoute v1 CRD và quyền tạo route của backend account. Staging bootstrap
không dùng kubectl hoặc sửa Gateway. Provider apply không chứng minh Accepted/
ResolvedRefs; chỉ báo browser checks nếu URL thực sự truy cập được.
