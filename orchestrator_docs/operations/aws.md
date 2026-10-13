---
id: RUNBOOK-AWS
artifact: operations-runbook
status: current
last_reviewed: 2026-10-13
---

# AWS verification

## Quyền và điểm dừng

[V2 acceptance runbook](terraform-v2-acceptance.md) sở hữu account/region,
small sizing, estimates, deadlines và cleanup. [M04](../iterations/M04-terraform-v2/README.md)
yêu cầu A01/E01/E02 pass, video và dự toán đã lưu, k8s-4f sạch trước AWS.
Quyết định 2026-10-14 bỏ điểm duyệt 2: Codex tự chạy I04-12 khi đủ điều kiện;
điểm duyệt bàn giao cuối vẫn giữ. Account 452025861381, region us-east-1.

## Preflight

1. Kiểm lại account/principal/region không hiển thị key. Host chain chỉ cho
   read-only preflight; product execution dùng Connection onboarding qua UI/Vault.
2. Ghi exact module/Terraform/provider/renderer pins, small compatible sizing,
   resource counts và giá hiện hành trong estimate table; không trần USD giả.
3. Run ID/tag project/owner/environment/run-id/expires-at/managed-by,
   original state/target/module/ownership handles sẵn cho independent supervisor.
4. Durable encrypted platform PostgreSQL HTTP state, khóa/fencing, Vault và
   EKS network/access requirements phải preflight được trước apply.
5. Terraform validate/plan review allowlist/count limits; UI Preview vẫn pure.
6. Kiểm `public_access_cidrs` của Definition EKS chứa IP ra Internet hiện tại
   của executor. Thiếu hoặc không xác định được IP thì dừng trước create;
   PE/Admin cập nhật danh sách IP `/32` qua cấu hình/Preview. Không tự thêm IP
   hoặc mở mọi IP. Private endpoint và IAM/RBAC vẫn bật.

## Thực thi và cleanup

V2-A02 dùng ElastiCache/Aurora, V2-A03 dùng Redis in-cluster EKS/Aurora.
Mọi AWS node cùng Environment account/region; module Kubernetes bind EKS
completed target. Declare bằng Console form, Preview/Deploy theo UI, rồi chạy
job flow app/Redis/PostgreSQL thực tế. Không thay UI bằng aws-verify.sh hoặc API.
Script cũ chỉ là checkpoint lịch sử tại a897d6b/3367f7e; code gaps ở IMP-021.

Cleanup ngay sau từng run, kể cả lỗi/cancel, reverse dependency qua đúng
original module/state/target. Gỡ consumer trước, application resources trước
namespace/EKS/VPC; dọn ECR/IAM/network phụ trợ thuộc run. Không account-wide
hoặc prefix wildcard delete, không xóa namespace để bỏ qua children states.
State/plan có thể chứa secret, chỉ giữ restricted ciphertext/private artifacts;
metadata/logs/manifests/evidence không để lộ raw secret values.

Trước nâng platform release, giữ module cache, manifest/binary đã pin và
provider plugins của các release còn tài nguyên. Khai báo các manifest lịch sử
qua [retained release configuration](../architecture/contracts/terraform-modules.md)
để destroy và absence probe dùng đúng pins cũ; không chạy cleanup bằng version
mới chỉ vì version cũ thiếu trên host. Giữ cấu hình này đến khi xác minh hết các
instance của release tương ứng; thiếu artifact vẫn là cleanup chưa hoàn tất.

Query AWS theo physical IDs/run tags: VPC/subnets/SG/routes/NAT/IGW, EC2/EBS/
EIP, EKS, RDS, ElastiCache, ECR, IAM/LB phát sinh. Destroy exit 0/empty state
không đủ absence proof. Remaining IDs phải được xử lý và xác minh; chưa sạch
thì run chưa hoàn tất và không bắt đầu run mới. Giữ failed video/assertions
và evidence cleanup để final handoff được người dùng duyệt.
