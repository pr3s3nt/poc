---
id: WORKLOAD-CATALOG-CONTRACT
artifact: operation-contract
status: draft
last_reviewed: 2026-10-28
---

# Workload catalog — contract đề xuất cho issue #24

[Implementation entry](../../implementation/workload-catalog.md) phân biệt
quyết định đã chốt, thiết kế đề xuất và khả năng đã có. Contract này chưa thay
ADR-024 hoặc Score public contract cho tới khi review thiết kế hoàn tất.

## WC-C01 — Mô hình và quyền

WorkloadTemplate là mẫu authoring có ID/version, label/description qua locale,
category (frontend/backend/worker/stateful/scheduled), runtime kind
(Deployment/StatefulSet/CronJob), input schema, defaults và capabilities.
Category không quyết định dependency. Backend không tự thêm database; MySQL
không tự thêm CronJob. Template không chứa secret bytes hoặc namespace cố định.
PE/Admin publish/retire version; Developer đọc và chọn trong Organization.
Version published immutable; retire không xóa version đang được draft/history dùng.

Đề xuất reuse Git render bundle inspection/cache và Definition matching hiện có.
Template chỉ cung cấp authoring schema và yêu cầu renderer capability; không
cho Developer chọn driver account/module hoặc vượt matching criteria. Preview
resolve Definition theo context, kiểm supported runtime/capabilities và pin
bundle/source hash + template version/hash. Không match thì báo lỗi actionable;
không tự fallback Deployment. Template mới không được tự thay cấu hình đã lưu.

V1 cần default templates frontend/backend/worker/stateful/scheduled, form generic
đọc schema với controls hữu hạn và typed validation; không tải JavaScript từ
catalog. Không hardcode logic theo tên Passbolt/MySQL. Template authoring/version
API là đề xuất mới cần review; không thêm route public mà không cập nhật contracts.

## WC-C02 — Desired workload và round-trip

Giữ Score cho trường Score hỗ trợ; fields mở rộng nằm trong versioned platform
extension `workloadExecution` (tên wire đề xuất). Không giả định parser Score hiện
có tự giữ extension. Một normalized desired model được dùng cho form, import,
Preview, renderer và persistence; không có bản JSON riêng chỉ UI hiểu.

| Nhóm | Kiểu dữ liệu/validation đề xuất |
|---|---|
| Template | immutable ID/version/hash; runtime enum; không secret |
| Container | unique name, image, command string[], args string[], workingDir tuyệt đối |
| Env | literal hoặc typed shared config/secret/resource-output/workload-Service ref; dùng UC-12/16, không literal secret |
| Resources | CPU/memory requests/limits theo từng container; giữ absence vs explicit values; validate request <= limit |
| Probes | startup/readiness/liveness; HTTP/TCP/exec union; period/timeout/threshold; port đã khai báo; không default endpoint không biết |
| Service | enabled, ClusterIP hoặc headless, unique named ports {port,targetPort,protocol}; port 1..65535; target container/port hợp lệ |
| Public route | service port ref, path/host theo policy route hiện tại; không đồng nghĩa Service enabled |
| Storage mount | resource ref, container ref, absolute mountPath, readOnly; không trùng/che protected mounts |
| Secret file | scoped secret ref + key/path/mode; approved delivery only; read-only; không inline content |
| Registry | reference đến cơ chế pull credential hiện hành; không literal password |
| StatefulSet | replicas, stable identity, serviceName từ governing Service; PVC identity độc lập |
| CronJob | schedule, IANA timezone, suspend, concurrency Allow/Forbid/Replace, startingDeadlineSeconds, job activeDeadlineSeconds/backoffLimit, success/failure history limits |

Runtime chỉ khai báo một lần; default có provenance template version. Mọi
field trên phải giữ qua Save/Edit/Preview/Deploy/restart. Trường không biểu diễn
được trong form giữ trong advanced mode và không âm thầm drop khi save.
Public Score/import behavior phải versioned; legacy workload không có extension
vẫn giữ hành vi hiện hành, không tự thêm Service/storage/probe/limits.
V1 đề xuất một replica cho stateful + single-claim; nhiều replica cần per-ordinal
claim hoặc storage chia sẻ và là capability riêng, không ngầm mount RWO cho mọi Pod.

## WC-C03 — Resource/storage binding

Database và persistent storage provision qua matched Terraform Definition.
PVC là resource có state/instance identity ổn định; workload renderer chỉ tham
chiếu claimName/namespace qua outputs, không đồng thời sở hữu PVC đó trong
inventory. Không adoption PVC cũ ngoài scope. Giữ module/state/UID/target guards.
Storage schema: size, access mode, StorageClass (do catalog publish), volumeMode;
outputs gồm claimName/namespace, không chứa credential. Mount phải cùng namespace
và target cluster; cross-namespace/cluster bị chặn.

Không đợi PVC Bound vô điều kiện trước khi apply consumer: StorageClass
WaitForFirstConsumer có thể Pending hợp lệ. Phân biệt provision-created với
consumer-ready; sau apply workload kiểm PVC binding và rollout trong deadline.
Provider/module phải cấu hình wait semantics tương ứng; không hạ readiness DB.

Update image/config giữ instance/PVC. Đổi StorageClass/access mode/target hoặc
thu nhỏ volume cần replacement/migration: Preview chặn và giải thích, không
tự destroy/recreate. Expand chỉ khi capability cho phép và có kiểm chứng.

Quyết định người dùng: remove workload giữ PVC. Last reference removed =>
UNREFERENCED theo resource lifecycle hiện có, không destroy. Xóa dữ liệu là
resource deletion có Preview/confirmation, original ownership/UID/state checks.
Chặn xóa Environment/namespace chứa storage giữ lại kể cả claim không nằm trong
workload inventory. Không dùng Terraform prevent_destroy như cơ chế duy nhất.
Dùng lại storage cùng Environment qua explicit binding, kiểm compatibility;
không tự gắn theo tên. Database PVC cũng thuộc chính sách bảo vệ dữ liệu này.

## WC-C04 — Rendering, Service và runtime

Deployment/StatefulSet giữ primary invariants hiện hành. CronJob phải được
bổ sung làm primary runtime thật, không tạo Deployment giả để qua guard.
Pod path cho CronJob là spec.jobTemplate.spec.template; inject ownership,
registry/env/secret references, security/probes/resources vào đúng path.
Score-generated Deployment nếu là intermediate phải được bundle chuyển đổi và
loại khỏi output cuối; guard kiểm một primary controller đúng loại đã pin.

Service named workload được tạo khi desired Service enabled; headless governing
Service có identity ổn định riêng nếu ứng dụng cần ClusterIP Service khác. Preview
hiển thị cả hai; không đổi Service immutable field bằng replace ngầm. Scheduled
job mặc định không Service; binding đến Service resource khác vẫn được phép.
CronJob/Job không được đưa vào rollout wait dành cho StatefulSet/Deployment.
Deploy CronJob thành công nghĩa cấu hình lịch đã được apply, chưa chứng minh Job
thành công. Theo dõi Job qua owner UID đến CronJob và Environment/workload,
last schedule, active Jobs, last result, history/logs. Không báo Next run chính xác
nếu không có evaluator cùng semantics/timezone; dùng unknown thay đoán.

Suspend chỉ ngừng lịch mới, không tự kết thúc Job đang chạy. Remove/update lịch
phải định nghĩa xử lý active Jobs trước implementation; đề xuất block delete khi
còn active Jobs, cho explicit cancel thuộc thao tác riêng. Retry update idempotent;
không tự tạo manual Job khi retry deploy.

## WC-C05 — Secret delivery

Tái sử dụng Vault KV + VSO/platform-managed Secret revision. Bytes không đi vào
Score/extension/template/Git/Preview/logs/Terraform variables hoặc public outputs.
Terraform module nhận tên Secret/key references, không dùng data source đọc bytes.
Secret resource auxiliaries có owner rõ, không để Terraform và VSO cùng sở hữu.
Database resource và CronJob cần scoped bundle/secret binding mới nếu đường hiện
có chỉ hỗ trợ workload env; không suy mặc nhiên đã có. Dependency graph phải
đưa secret delivery trước consumer mà không tạo vòng DB-output → DB-credential.

File projection lấy đúng approved key từ Secret bundle; volume refs/mode được
platform cấp, không cho bundle tự lấy Secret bất kỳ. Khi ứng dụng cần import GPG,
entrypoint đọc file mount, không ghi khóa vào image/log. Revision update tạo
Preview/deploy; với Job đang chạy giữ revision cũ đến hoàn tất. Chỉ dọn secret
revision khi không còn live consumer. Registry refs được scope/validate như hiện tại.

## WC-C06 — API/persistence đề xuất

Mở rộng workload GET/save hiện hành bằng execution extension + template pin,
giữ CAS draftVersion và existing error envelope. Catalog read cần scoped
list/detail versions; publish/retire cần PE/Admin và CAS. Concrete paths/schema
phải được duyệt ở API-mapping trước coding, không coi ví dụ này là endpoint đã có.

Database design cần cập nhật canonical schema/ERD trước migration: template
version records immutable scoped org; desired pin/extension trên workload draft;
normalized snapshot/pin trên deployment set, plan và applied instance; không chỉ
lưu trong browser. Giữ metadata không chứa secret; không sửa lịch sử cũ khi đổi
catalog. Memory/JSON/Postgres adapters cùng semantics. CronJob status là read
projection theo applied identity; không suy từ desired draft.

## WC-C07 — Preview và compatibility

Preview không provision hoặc resolve secret bytes. Pin template/bundle/draft,
config revision, dependency/target snapshot; thay đổi bất kỳ làm token stale.
Hiển thị controller, Service/route/ports, mounts/PVC create/reuse/retain,
resource dependencies, cron schedule/timezone/concurrency và immutable changes.
Giữ symbolic outputs khi PVC/DB chưa tạo. Runtime admission kiểm lại pin/target/
versions; không overwrite drift hoặc catalog mismatch để cố deploy.
