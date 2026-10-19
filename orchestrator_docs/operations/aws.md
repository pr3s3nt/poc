---
id: RUNBOOK-AWS
artifact: operations-runbook
status: current
last_reviewed: 2026-10-19
---

# AWS verification

## Quyền và điểm dừng

[V2 acceptance runbook](terraform-v2-acceptance.md) sở hữu account/region,
small sizing, estimates, deadlines và cleanup. [M04](../iterations/M04-terraform-v2/README.md)
yêu cầu A01/E01/E02 pass, video và dự toán đã lưu, k8s-4f sạch trước AWS.
Quyết định 2026-10-14 bỏ điểm duyệt 2: Codex tự chạy I04-12 khi đủ điều kiện;
quyết định 2026-10-15 bỏ điểm duyệt cuối, hoàn tất gửi báo cáo. Account 452025861381, region us-east-1.

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
Mỗi AWS node dùng account/region của winning Definition theo ADR-017; fixture
hiện hành do PE cấu hình cùng account/region. Module Kubernetes bind đúng EKS
completed target từ resource reference, không dùng account Environment. Declare bằng Console form, Preview/Deploy theo UI, rồi chạy
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
và bằng chứng cleanup để báo cáo cuối, không chờ duyệt.


## V04–V06 refactor execution — 2026-10-18

User requests continuous V04→V05→V06 under issue #4. Existing permissions above
remain limited to452025861381/us-east-1 and owned test resources; there is no
new account/region, retained-resource deletion or reset authority. Follow
[ADR-017](../architecture/decisions/ADR-017-definition-dependency-execution.md)
and the [coverage/handoff](../verification/2026-10-18-v040506-handoff.md).
V04 must pass full real app/review/checks/video/cleanup before V05. Current AWS
admission consumes the completed integrated main-flow V04 evidence and its
final post-cleanup runner receipt (version2, MAIN_FLOW_2026_10_19). E01/E02 are
excluded by the19-10 user decision; it does not require renamed or repeated
historical M04 scenarios. An incomplete browser PASS cannot admit AWS.
The coordinator verifies native video and commits/pushes the full V04 milestone
before starting V05. Historical M04 receipts remain provenance, not replacement
source runtime evidence. V05 requests
EKS through authored graph and proves replacement before AWS profile retirement.
V06 runs A02 then independently proven cleanup then A03, followed by gated final
executionProfile retirement and impacted final-source validation. No approval
between gates when existing decisions/permissions apply.

Reverify STS/account/region and exact counts/prices/sizing/deadlines for each run;
no fixed USD ceiling has been granted or invented. Unknown actual cost stays
“chưa xác định”. EKS standard support version/AMI/addon availability, egress /32,
API access/node capacity, service-linked role preservation and native service API
absence must be checked on that run. Historical M04 passes do not establish
replacement-source runtime correctness. Preserve recovery on uncertain cleanup.

## Run-owned recovery material after host restart

V05/A02/A03 evidence, encrypted PostgreSQL files/state key, original manifests,
pinned cleanup binaries and run-owned Vault file storage/init material stay in
private0700 directories under /var/tmp until native service absence is proven.
The Connection/output Vault uses file storage rather than dev/in-memory storage;
its root/unseal material is0600 and never printed, uploaded or committed. Scoped
product tokens cover execution+cleanup plus one hour local recovery margin;
this does not extend cloud resource deadlines or cost estimates.

After a host restart, do not replay deploy or reset SQL/Vault: restore the same
PostgreSQL instance and original Vault configuration/port, unseal from its private
init material, verify original identity/ownership, then continue the recorded
cleanup manifest with pinned binaries. The independent supervisor covers runner/
browser loss; a whole host restart needs this manual recovery before dependent
work. Keep all supporting files when absence remains unknown. A normal local
Vault restart/read-back check validates storage before first AWS create.
