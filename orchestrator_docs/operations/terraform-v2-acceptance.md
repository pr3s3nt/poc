---
id: RUNBOOK-TERRAFORM-V2-ACCEPTANCE
artifact: operations-runbook
status: current
last_reviewed: 2026-10-13
related: [ADR-014, UC-03, UC-04, UC-05, UC-06, UC-08, UC-16]
---

# Terraform v2 — acceptance và quyền thực thi

Runbook này ghi scope người dùng đã chốt và execution procedure cần implement;
không khẳng định runner/modules/Redis đã có. ADR nằm tại
[ADR-014](../architecture/decisions/ADR-014-terraform-v2-lifecycle.md).

## Quyền được giao

- AWS target dự kiến: account `452025861381`, region `us-east-1`; STS read-only
  ngày 2026-10-13 trả principal `arn:aws:iam::452025861381:user/itadminaws`.
  Người dùng cho kiểm tra credential và thực thi v2 theo target phát hiện;
  preflight từng run phải xác minh lại, account/region khác phải hỏi lại.
  STS success chỉ chứng minh authentication, không chứng minh provisioning IAM.
- Cho tạo/đọc/update/xóa hạ tầng test mới: VPC/subnets/routes/IGW/NAT/security
  groups, EKS/node groups/access/IAM roles-policy attachments cho chính run,
  ElastiCache/Aurora cùng subnet/parameter groups và dependencies cần thiết.
  ECR/images test được phép khi cần delivery; không thay account-wide settings,
  existing role/policy, shared network hoặc tài nguyên có sẵn.
- k8s-4f: chỉ tạo/xóa run-owned namespace `orch-v2-test-<run-id>` và objects test
  trong đó. Không sửa/xóa cluster, existing namespaces/PVC/apps, Vault/VSO,
  storage classes hoặc ingress controller có sẵn. Thiếu cluster capability
  cần hỏi trước khi cài/sửa thành phần cấp cluster. PV/EBS do PVC test tự tạo
  được thu hồi nếu chứng minh đúng ownership; không xóa PV ngoài run.
- Cho dùng platform credential store, existing Connection/store qua public UI
  với scoped credentials; fixture metadata mới trong state test cô lập. Không
  reset retained database hoặc xóa user-owned metadata.
- Không trần USD cố định; small compatible sizing, cost estimate trước run,
  resource-count/allowlist plan review và deadline hữu hạn bắt buộc. Không tự
  ghi một budget ceiling là người dùng đã chốt. Chạy tuần tự; xóa ngay sau mỗi
  run dù thành công/thất bại. Không mở acceptance update/delete matrix.

## Preflight và deadlines

1. Ghi run ID duy nhất, target/account/region, principal, module/binary/provider
   pins và estimated per-hour/per-run cost bằng nguồn giá hiện hành. Giới hạn
   resources/counts và small sizing đủ tương thích; không hạ acceptance để giảm giá.
2. Xác minh platform PostgreSQL/state encryption key, Vault credentials/store,
   immutable module cache, score-k8s binary và allowed providers. Credentials
   không xuất hiện trong output. STS host chain là preflight; product execution
   phải resolve Connection được onboarding qua UI, không host fallback.
3. Read-only permission/capability checks và Terraform validate/plan review.
   UI Preview vẫn pure, không bị thay bằng Terraform plan hoặc script deployment.
4. Run manifest private bên ngoài repository ghi `project`, `owner`, `environment`,
   `run-id`, `expires-at`, `managed-by`, states/physical intent/dependency/ownership.
   Kubernetes namespace có corresponding labels/annotations; stable instance
   names/state không thay theo retry run ID.
5. Trước create, khởi chạy cleanup supervisor độc lập process group của runner/
   browser, có credential/state handles cần thiết, EXIT/signal hooks và expiry
   watchdog. Test supervisor kill-runner/browser path bằng owned low-cost fixture.
6. Initial timeout policy cho acceptance: k8s-4f execution tối đa 60 phút;
   AWS execution tối đa 180 phút, cleanup bắt đầu ngay khi xong/lỗi/timeout.
   Provision timeout từng node nằm trong deadline tổng; cleanup budget ban đầu
   120 phút, hết budget thì escalation/manual continuation, không bỏ tài nguyên.
   Ghi `expires-at` từ deadline execution; đây là trigger cleanup, không cam kết
   provider delete lập tức. Deadline tuning phải ghi trước run, giữ hữu hạn và
   không kéo dài run lỗi để tạo thêm resources.

## Acceptance app và nghiệp vụ

Dùng `backend/examples/acceptance-app/` gồm frontend, backend, worker hiện có.
Giữ job submission/result UI; không thêm màn hình kiểm chứng riêng. PostgreSQL
shared giữa backend/worker; Redis shared giữa backend/worker trong Environment.
Worker persist job/result vào PostgreSQL, rồi write-through Redis cache theo
job ID. Backend đọc cache trước, fallback PostgreSQL khi cache miss, ghi lại
cache khi đọc từ DB. Worker/reader dùng contract host/port/auth/tls đã bind.

Redis required trong v2 scenario: unavailable/auth/binding sai báo failure trong
existing diagnostic/readiness và assertions; fallback cache miss không biến
Redis outage thành pass. PostgreSQL là nguồn dữ liệu bền vững. Failure cache
write sau DB commit giữ kết quả DB và báo degraded/failure an toàn, không tạo
job duplicate hoặc giả success Redis. Cache key có prefix app/env/run/job, TTL
ngắn do app config để assertions có thể prove miss/fallback và refill. Không
log values/password; no new diagnostic screen hoặc API bypass UI business step.

## Ba happy paths

| ID | Target | Dependency class / implementation |
|---|---|---|
| V2-A01 | k8s-4f | Redis/PostgreSQL `in-cluster`, Terraform namespace/cache/db |
| V2-A02 | AWS managed | Terraform VPC/EKS/namespace, Redis `cloud-managed` ElastiCache và PostgreSQL `cloud-managed` Aurora |
| V2-A03 | AWS kết hợp | Terraform VPC/EKS/namespace, Redis `in-cluster` trên EKS và PostgreSQL `cloud-managed` Aurora |

Mỗi scenario chạy hai chặng headed browser liên tục hoặc có phase/video liên kết:

1. Chính Web Console: sign-in, đăng ký/verify Connections cần thiết, chuẩn bị
   Resource Types/Definitions/module pins/provider bindings bằng UI, chọn
   Application/Environment, thêm/sửa ba Workloads bằng form, chọn image/params/
   classes, shared dependency IDs và output/secret/Service bindings, Save draft.
   Nếu reuse Connection/Definition tạo qua UI ở phase chuẩn bị thì evidence
   phải liên kết phase đó; không seed catalog qua API để thay bước nghiệp vụ.
2. Xem trước thay đổi: assertions đúng Definition/module revision, class,
   target/account/region và provider-first graph, score-k8s bundle. Deploy qua UI,
   quan sát persisted Terraform stages, workload Ready và kết quả không secret.
3. Mở frontend vừa deploy: submit job qua existing form, chờ worker xử lý tới
   completed, verify result đúng input theo nghiệp vụ hiện có và reload vẫn
   thấy cùng job/result. No API request dùng để tạo/hoàn tất job thay thao tác UI.
4. Assertions kỹ thuật read-only/private ngoài bước nghiệp vụ: cả ba workload
   Ready đúng target; one shared PostgreSQL/Redis instance; provider state/objects
   và bindings đúng; actual PostgreSQL row/result, worker Redis write và backend
   cache hit. Đợi cache TTL hết, mở/tải lại result bằng UI, verify cache miss
   fallback đọc cùng kết quả PostgreSQL và refill Redis. Chỉ đọc evidence safe
   IDs/counts/status; không dump data/credential vào video/log công khai.

Acceptance app UI business assertions là điều kiện pass; chỉ ping dependencies,
Pod Ready hoặc Services gọi được nhau không đủ. Tất cả dùng adapter thật.
Có thể port-forward app/frontend cho browser; không cần public LB/DNS/TLS.

## Hai exceptions

| ID | Trigger và assertions |
|---|---|
| V2-E01 | Module input sai/missing kiểm tra được trước execution (ví dụ bounds/required của param hợp lệ trong schema); thao tác form/Preview báo đúng field và không tạo hạ tầng, no state apply/provider create |
| V2-E02 | Failure có kiểm soát ở Redis in-cluster sau namespace đã tạo: Definition test pin image reference đúng cú pháp nhưng digest không tồn tại, StatefulSet không Ready và node readiness hết timeout; quan sát đúng Terraform node/stage FAILED, giữ state/provenance, cleanup CREATED_BY_RUN và verify absence; sentinel preexisting namespace/resource ngoài run vẫn còn |

E02 không dùng crash/unknown acknowledgement để ép implement D12/D13 và không
simulate provider bằng fake. Fault image là executor/module setting của Definition
test đăng ký qua UI, không yêu cầu Developer nhập credential hoặc đổi nghiệp vụ
app. Fixture chỉ tồn tại trong test namespace, có pin/timeout/cleanup riêng;
không sửa cluster-wide admission/RBAC hoặc existing Connection. Dùng registry
được phép và digest xác minh không tồn tại, không outage ngẫu nhiên. Local shape
validation phải pass trước namespace create; failure được quan sát ở real pod
readiness và Terraform node. Không tái hiện được thì sửa fixture, không giả pass.

## Retry, cleanup và evidence

Scenario có tối đa một lần retry sau sửa lỗi, cleanup lần trước xong mới tạo
run mới. User-owned/preexisting/updated resource không bị xóa vì run failed.
Sau retry fail thì stop scenario và cleanup, báo chưa đạt. Cleanup failure chỉ
retry bounded calls/continue operator để thu hồi owned assets; không mở run mới.

Gỡ workload delivery trước, destroy Terraform application resources -> namespace
-> EKS -> VPC theo actual reverse graph, dọn run-owned ECR/IAM/network phụ trợ.
Không delete namespace trước state-owned children để bỏ qua lifecycle. Query
AWS physical IDs/run tags cho EKS, RDS, ElastiCache, VPC/subnets/SG/routes/NAT/
IGW, EC2/EBS/EIP, ECR, IAM và LB nếu phát sinh; Kubernetes namespace/objects/PV
owned được xác minh không còn. Empty Terraform state hoặc destroy exit 0 chưa đủ.

Mỗi run giữ headed video có cursor/click/typing/pauses đủ đọc, phụ đề Việt không
che UI; phase marks PREPARE/FORM/PREVIEW/TERRAFORM/READY/JOB/CACHE/CLEANUP,
assertions JSON, pin/estimate/preflight safe manifest, physical IDs và cleanup
queries. Giữ failed video/assertions trước retry, review/decode video và secret
redaction trước handoff. Evidence ở private directory ngoài Git; dated Markdown
verification links tới artifact handles và phân biệt live/fake. Cleanup pending
thì scenario/run chưa hoàn thành, v2 chưa đủ acceptance.
