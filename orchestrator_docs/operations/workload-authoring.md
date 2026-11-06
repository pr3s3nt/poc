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

## Giá trị hiệu lực và mặc định

Workload mới từ form dùng `workload-defaults-v1`. Workload cũ giữ policy cũ tới
khi chủ động đổi. Giá trị hiển thị lấy từ backend: Tùy chỉnh > Theo template >
Theo môi trường/nền tảng. Khôi phục mặc định xóa override; không ghi một bản sao
của giá trị hiện tại. Metadata lỗi thì giữ draft, chỉnh trường lỗi và thử lại.
Không đổi template/policy để chữa một lỗi runtime mà chưa kiểm tra Preview.

App container: staging 100m/128Mi request, production 200m/256Mi; memory limit
512Mi, CPU limit chưa đặt. Init/Sidecar dùng cấu hình riêng. Legacy không policy
vẫn dùng request đã nhập, nếu thiếu lấy limit, nếu cả hai thiếu dùng 10m/32Mi.
Giới hạn phải đủ request; profile ứng dụng lớn cần bộ tài nguyên phù hợp.

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
phép kiểm tra hợp lệ. Normal Init không có probes. V1 period/timeout/failure của
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

## Thứ tự form theo tần suất chỉnh sửa

Image và biến môi trường nằm đầu phần container; tiếp theo là CPU/RAM, volumes,
cổng, health checks, khởi chạy nâng cao và bảo mật. Replica nằm trong Mở rộng &
cập nhật. Kiểu thực thi, policy defaults, class và annotations nằm trong Cấu hình
ít thay đổi ở cuối form. Biến môi trường mở sẵn; CronJob có thêm Lịch chạy mở sẵn.
Các nhóm còn lại thu gọn với tóm tắt, mở ra khi cần chỉnh hoặc khi có lỗi.
