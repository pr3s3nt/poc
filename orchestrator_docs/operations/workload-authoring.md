---
id: OPS-WORKLOAD-AUTHORING
artifact: operations-runbook
status: current
last_reviewed: 2026-11-06
---

# Thêm và sửa Workload

Hướng dẫn này cập nhật phần liên quan của [issue #36](https://github.com/pr3s3nt/poc/issues/36)
cho [issue #37](https://github.com/pr3s3nt/poc/issues/37). Tính năng chỉ khả dụng
khi Definition đã chọn trỏ tới source công bố capability tương ứng; source cũ
không tự được nâng cấp. Kết quả kiểm chứng nằm trong verification, không suy từ
việc có control trên form.

Mở Application, chọn staging/production, thêm Workload rồi chọn template hoặc tự
cấu hình. Nhập tên và image nếu template chưa cung cấp. Save chỉ lưu desired draft;
Preview rồi Deploy là hai bước riêng. Variable/Secret tạo bằng hộp thoại riêng
được commit ngay; Cancel workload không xóa chúng.

Template Backend API không cung cấp image mặc định. Nhập image của ứng dụng vào
container trước khi tính cấu hình hiệu lực. Form hiển thị yêu cầu bổ sung image,
không yêu cầu Retry cho trường còn thiếu. Template có image được công bố sẵn vẫn
hiển thị image kế thừa và không ghi nó thành override khi chỉ mở/lưu.

## Giá trị hiệu lực và mặc định

Theo [ADR-038](../architecture/decisions/ADR-038-template-owned-workload-defaults.md),
không còn policy mặc định trong backend hoặc dropdown chọn policy. Giá trị bạn
nhập được ưu tiên; trường thiếu lấy từ `runtimeDefaults` của template đã ghim.
Nếu có `environments.production`, template đó tự công bố các giá trị production.
Backend chỉ ghép và kiểm tra hợp lệ, không bổ sung một bộ CPU/RAM/rollout khác.

Giá trị mặc định hiện mờ trong ô trống. Gõ trực tiếp để đặt giá trị riêng, không cần
xóa default; xóa giá trị riêng hoặc Khôi phục mặc định để kế thừa lại. Form không
hiện nhãn nguồn. Save giữ override và snapshot profile đã xác minh cùng pin template.
Đổi default cần publish version template mới, chọn chủ động và kiểm tra Preview.

Các template chuẩn 1.1.0 công bố App requests staging 100m/128Mi, production
200m/256Mi, memory limit 512Mi và không có CPU limit. Đây là dữ liệu template, không
phải constants backend. Profile khác có thể dùng giá trị khác. Tự cấu hình không
chọn template thì backend không cấp profile authoring; các fallback renderer nằm
trong file source bundle. Snapshot cũ có `workloadExecution.defaults` không còn được
chấp nhận; dữ liệu test cũ của task đã được phép xóa, không tự chuyển policy.

## Nhóm Workload và Container

Thông tin chung giữ tên, class và annotations. Class/annotation nâng cao vẫn
thuộc workload; annotation không mặc nhiên được chép sang Pod. Kiểu thực thi
lấy từ template; tự cấu hình có thể chọn controller phù hợp. StatefulSet vẫn chỉ
1 replica. CronJob dùng lịch chạy, không dùng replicas.

Mỗi container có Khởi chạy, Biến môi trường, Volumes & files, Tài nguyên tính toán,
Kiểm tra sức khỏe, Bảo mật container và Cổng container. Nhóm thu gọn giữ draft;
mở nhóm cần dùng. App/Init/native Sidecar và thứ tự khởi tạo giữ nguyên semantics.
Command/args mỗi dòng một đối số; để trống dùng image, không tự chạy shell.

## Bảo mật

Pod security chứa UID/GID, fsGroup, supplementalGroups và seccomp của Pod.
Container security chứa UID/GID, capabilities, filesystem chỉ đọc và quyền của
container. Không áp cùng UID cho mọi image. ServiceAccount do nền tảng/secret
delivery quản lý; tên có hiệu lực xuất hiện trong Preview. Token mặc định false;
Vault Agent cần token và không chấp nhận override false xung đột. Môi trường có
thể từ chối quyền/capability vượt policy; không tự bypass để deploy.

Profile `nginx-readonly` dùng image Nginx unprivileged, cổng 8080, readiness `/`,
filesystem chỉ đọc và hai emptyDir `/var/cache/nginx`, `/tmp`. Arbitrary image
không tự nhận non-root/read-only. Chỉ dùng profile với image tương thích.

## Cập nhật và khả dụng

Deployment stateless dùng RollingUpdate maxUnavailable 0/maxSurge 1. Persistent
mount giữ Recreate. StatefulSet dùng RollingUpdate partition 0 hoặc OnDelete;
không có maxSurge. Revision history staging 3, production 10; explicit 0 giữ nguyên.
PDB mặc định chưa bật. Khi bật cần Deployment stateless ít nhất 2 replicas và chọn
một minAvailable hoặc maxUnavailable (số nguyên hoặc phần trăm). PDB chỉ điều tiết
eviction tự nguyện; không thay rollout hay đảm bảo zero downtime. AlwaysAllow
phụ thuộc hỗ trợ của cluster, được server admission kiểm trước publication.

## Volumes và StatefulSet

Thêm volume/file tại container cần dùng. Picker ưu tiên nguồn sẵn có; tìm/lọc
trước khi tạo nguồn mới. Mount path/subPath/readOnly thuộc từng mount. Gỡ mount
không xóa PVC/resource/Secret. PVC dùng chung giữ identity, emptyDir chỉ trong Pod.
ConfigMap/Secret luôn chỉ đọc; Secret file mặc định 0400. hostPath mặc định
Directory/read-only, đường dẫn thuộc node. emptyDir mới gợi ý 256Mi.

Claim template StatefulSet tạo claim riêng theo Pod, khác PVC resource dùng chung.
Nhập tên, size, access mode, StorageClass nếu cần và mount vào container. Retain
khi xóa và scale down; tên/config claim bất biến sau deploy. Workload removal
không xóa claim; namespace/Application cleanup sẽ bị giữ nếu còn claim ngoài
inventory xóa đã được chứng minh. Không có chức năng tự adopt claim cũ: cần quy
trình migration/cleanup dữ liệu riêng có quyền và UID proof.

## Health, mạng và lịch

Không tự đoán probe endpoint/cổng. Chỉ bật readiness/liveness/startup khi đã có
phép kiểm tra hợp lệ. Normal Init không có probes. Template chuẩn 1.1.0 khai báo period/timeout/failure của
readiness/liveness là 10/2/3; startup 5/30, success 1. Startup có mặt thì initial
delay mặc định 0. Input trống giữ kế thừa.

Service chỉ có khi người dùng/template công bố cổng hợp lệ. Có thể tắt Service
kế thừa. Port/target port chỉ ra đường kết nối; tạo Service không công khai Internet.
CronJob bắt buộc schedule, timezone kế thừa Asia/Bangkok, Forbid, backoff 0,
history thành công/thất bại 3; không đặt deadline nếu chưa biết yêu cầu tác vụ.

Import chỉ nhận Generic Workload YAML/JSON; manifest Kubernetes tùy ý không phải
định dạng nhập. Preview cần kiểm giá trị/nguồn, rollout, quyền Pod, PDB, claim,
retention và diff trước Deploy. Nâng Definition/source cho Environment là hành
động riêng trong Settings → Phiên bản tài nguyên; workload đang chạy giữ pin cũ.

## Form cơ bản và nâng cao

Tên workload, tên/image container và biến môi trường hiện ở phần cơ bản. Các nhóm
CPU/RAM, volumes, cổng, probes, khởi chạy và bảo mật nằm trong Hiện nâng cao của
container. Service, dependencies, replica/cập nhật, Pod security và cấu hình ít
thay đổi nằm trong Hiện nâng cao Workload. CronJob vẫn hiện Lịch chạy ở ngoài.
Đóng/mở nâng cao giữ phần đang nhập; lỗi validation mở phần cần sửa.

## Khai báo defaults trong template

Ví dụ một mục trong `workloadTemplates` của bundle:

```yaml
- id: backend-small
  version: 1.0.0
  kind: Deployment
  labels: {vi: Backend nhỏ, en: Small backend}
  inputsSchema: {type: object, additionalProperties: false, properties: {}}
  defaults: {} # Mặc định template inputs, tách khỏi runtimeDefaults.
  runtimeDefaults:
    resources:
      requests: {cpu: 250m, memory: 256Mi}
      limits: {memory: 768Mi}
    replicas: 1
    execution:
      revisionHistoryLimit: 3
      updateStrategy:
        type: RollingUpdate
        rollingUpdate: {maxUnavailable: 0, maxSurge: 1}
      persistentUpdateStrategy: {type: Recreate}
    environments:
      production:
        resources:
          requests: {cpu: 400m, memory: 512Mi}
        execution: {revisionHistoryLimit: 10}
```

Source phải công bố `template-defaults-v1` và các capability runtime đang dùng.
Image vẫn do người dùng nhập nếu template không cung cấp. Khai báo probe endpoint,
non-root/read-only và writable paths chỉ khi ứng dụng thực sự hỗ trợ. Các defaults
PDB/probe/claim không tự bật hay tạo tính năng chưa được chọn.
