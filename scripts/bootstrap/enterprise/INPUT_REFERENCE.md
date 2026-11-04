# Chú thích input staging

File chạy là JSON chuẩn nên không có comment nội tuyến. Dùng tài liệu này cùng
`staging.input.json`; token/mật khẩu nằm trong file riêng, JSON chỉ chứa đường dẫn.
Không copy địa chỉ/IP/credential hoặc commit demo của cá nhân sang công ty.

| Trường | Cần điền / ý nghĩa |
|---|---|
| `environment` | Danh tính target/state, ví dụ `company-staging`; đổi target phải dùng state khác |
| `parameters.definitionPrefix`, `definitionName` | Prefix ID và tên hiển thị cho sáu Definitions; chọn một lần rồi giữ nguyên |
| `parameters.targetClass` | Class cluster/workload và resource references tương ứng |
| `parameters.environmentType` | Loại Environment để matching; preset là `staging` |
| `parameters.backendStorage` | Dung lượng PVC backend, ví dụ `1Gi`; thay đổi sau deploy là thay đổi cấu hình, không tự upgrade |
| `parameters.inputsRepository`, `inputsCommit` | HTTPS repo + full commit chứa `platform-resources/` |
| `parameters.modulesRepository`, `modulesCommit` | HTTPS repo + full commit chứa Terraform modules |
| `parameters.templatesRepository`, `templatesCommit` | HTTPS repo + full commit chứa workload templates |
| `parameters.*ModulePath` | Đường dẫn module trong repo; namespace/PostgreSQL/Redis/persistent-storage |
| `parameters.moduleSourceId`, `*ModuleId`, `*ModuleSHA256`, `templateTreeSHA256` | Helper lock tính; không đoán/copy ID từ repo demo. Contract/packaged recognition phụ thuộc đúng release đang chạy |
| `console.url`, `username`, `passwordFile` | Origin Console và account Admin/PE đã có; mật khẩu là file owner-only |
| `developer` | Account Developer thật, cùng Console/Organization; bootstrap không tự tạo user |
| `platform.gitAccess.url` | Scope HTTPS của Organization GHES, ví dụ `https://github.company.example/platform` |
| `platform.gitAccess.username`, `tokenFile` | Bot và đường dẫn token; quyền source + CD repository provisioning |
| `platform.gitAccess.caPemFile` | PEM CA cho Git/GHES; nếu dùng public-trust cert thì bỏ trường này, không để file rỗng |
| `platform.gitAccess.repositoryUrl`, `ref` | Kiểm tra connectivity bằng repo inputs và exact commit đã khai báo |
| `platform.account` | Tên account, context, server endpoint và kubeconfig. Endpoint phải tới được từ backend, không dùng loopback của laptop |
| `platform.secretStore` | Tên Vault, URL backend/workload nhìn thấy, KV-v2 mount, Kubernetes auth mount, tokenFile, CA nếu cần |
| `platform.delivery` | Tên Fleet binding, platform Git và default. `config` để discovery tự điền hoặc đặt đúng `workspace`/`targetCluster` nếu có nhiều lựa chọn |
| `platform.resourceTypes`, `definitions`, `namespaceAccess.rulesFile` | Git document refs; helper điền SHA-256. Không đổi thành local/inline để tránh lỗi fetch |
| `platform.templateSources` | Git path của template bundle, commit và tree digest |
| `application.name`, `subdomain` | App nghiệm thu riêng, tránh đụng app đang có |
| `application.environment` | `staging` hoặc `production` của mô hình sản phẩm; không đồng nghĩa cấp quyền target |
| `application.images` | Ba images từ registry nội bộ, immutable tag hoặc digest; không `latest` |
| `application.variables`, `secretFiles` | Dữ liệu nghiệm thu và file secret; giữ đúng hợp đồng acceptance workload |
| `verification.url` | URL thật trỏ frontend acceptance app sau deploy, nhìn thấy từ host Playwright |
| `verification.forward` | Tùy chọn: chỉ dùng nếu chủ động chọn port-forward. Preset công ty không giả định có route/forward |
| `verification.storage.value` | Chuỗi kiểm thử công khai ghi vào PVC; không dùng dữ liệu thật/bí mật |
| `runtime.mode` | `check` → `bootstrap` hoặc `all`; mode nằm trong file, không qua override ngầm |
| `runtime.stateDir`, `reportDir`, `gitCacheDir` | Đường dẫn riêng từng target, ngoài Git; giữ lại để resume/lũy đẳng |
| `runtime.clusterKubeconfigFile` | Kubeconfig cho native read-back/scoped namespace setup của script |
| `runtime.recordVideo` | Mặc định false ở staging; nếu bật, chỉ các checks thành công mới lưu video workload; không quay secret-entry |

Tất cả đường dẫn tương đối được tính từ vị trí file input; `~/` chỉ dùng với
đường dẫn file/directory được loader hỗ trợ. Khóa file cùng thư mục để không đổi
nghĩa các đường dẫn. `LOCK_WITH_HELPER` và `REPLACE_*` phải được thay trước khi chạy.
Với custom modules/template catalogs, kiểm tra input/output schema và workload
TemplatePin riêng; thay repository/commit không tự làm contract tương thích.
