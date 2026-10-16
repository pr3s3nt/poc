---
id: VERIFY-M04-COMPLETION-20261016
artifact: verification-record
status: evidence
last_reviewed: 2026-10-16
related: [M04, I04-06, I04-07, I04-08, I04-09, I04-10, I04-11, I04-12, IMP-021, ADR-014]
---

# M04 — bàn giao hoàn tất I04-06 đến I04-12

Phạm vi được giao đã hoàn tất. Năm kịch bản đều có biên nhận PASS, exit0,
CLOSED_CLEAN; Codex đã đối chiếu bằng chứng độc lập. Không còn tài nguyên
cloud/cluster hoặc fixture PostgreSQL do các lượt này tạo. Kết quả FAIL của
các lượt trước vẫn giữ nguyên cùng video và biên bản phục hồi.

## Kết quả từng task

| Task | Kết quả | Bằng chứng |
|---|---|---|
| I04-06 | AWS Connection qua Console/Vault, guard quyền và scope; 1082 Go, 479 frontend đạt | [Biên bản](2026-10-14-i04-06-aws-connection.md) |
| I04-07 | Definition/source/class, Preview và renderer; 1121 Go, 549 frontend đạt | [Biên bản](2026-10-14-i04-07-definition-workload-ui.md) |
| I04-08 | Xóa selected-set qua Console, quyền và dependency; 1171 Go, 609 frontend đạt; A01 bổ sung chứng cứ xóa thật | [Biên bản](2026-10-14-i04-08-resource-delete-ui.md) |
| I04-09 | Redis acceptance app, PostgreSQL/Redis thật, ảnh amd64/arm64 theo digest; 1254 Go, 609 frontend đạt | [Biên bản](2026-10-14-i04-09-acceptance-redis.md) |
| I04-10 | Runner, encrypted state, durable outputs, supervisor độc lập; 1351 Go/race, 622 frontend đạt | [Biên bản](2026-10-14-i04-10-integration.md) |
| I04-11 | A01/E01/E02 trên k8s-4f đạt; video, SQL sau restart, dự toán và cleanup độc lập | [Biên bản](2026-10-14-i04-11-k8s4f.md) |
| I04-12 | A02/A03 AWS thật đạt; job/cache/TTL, video/SQL, cleanup và đối chiếu tài nguyên giữ lại | [Biên bản](2026-10-15-i04-12-aws.md) |

Code/test do Claude và agent gpt-6-luna thay thế khi Claude hết hạn mức viết;
Codex review, kiểm tra độc lập, commit/merge/push và chạy thật. Cloud session
`session_01HtXzgfyBzoNma2DQ2SzNY5` và lịch sử bàn giao được giữ trong M04;
không gửi credential, state hoặc kubeconfig vào cloud. Không còn nhánh remote
`claude/m04-*` của công việc này sau merge.

Video local I04-06..08 có SQL/Vault/Terraform CLI thật nhưng một số transport/
executor/readiness giả lập, được ghi rõ trong từng biên bản; không dùng chúng
thay bằng chứng live I04-11/12. I04-09/10 là kiểm chứng unit/CLI nên không có
video UI riêng. Luồng browser ứng dụng được kiểm trên ba happy path bên dưới.

## Năm kịch bản thật

| Kịch bản | Lượt đạt / source | Kiểm tra / mốc | Video phụ đề | Kết quả |
|---|---|---|---|---|
| A01 | a01-20261015-r4 / d1f1da4 | 42 / 8 | 1339,07 giây | PASS/exit0/CLOSED_CLEAN |
| E01 | e01-20261015-r2 / 05bba6e | 12 / 4 | 484,73 giây | PASS/exit0/CLOSED_CLEAN |
| E02 | e02-20261015-r3 / cdfb027 | 15 / 5 | 687,87 giây | PASS/exit0/CLOSED_CLEAN |
| A02 | a02-20261016-r2 / b083d97 | 35 / 8 | 6677,13 giây | PASS/exit0/CLOSED_CLEAN |
| A03 | a03-20261016-r2 / a121642 | 35 / 8 | 4974,20 giây | PASS/exit0/CLOSED_CLEAN |

A01: ứng dụng/PostgreSQL/Redis trên k8s-4f; xóa Workload rồi PE chọn thêm
PostgreSQL dependency, namespace bỏ tick còn READY trước safety net.
E01: input sai bị chặn trước execution, không state apply/provider create.
E02: Redis image lỗi thật, PVC Bound/ErrImagePull/readiness timeout; ghi đúng
failure và dọn tài nguyên, giữ lịch sử FAILED.
A02: EKS + Aurora + ElastiCache. A03: EKS + Aurora + Redis in-cluster,
PVC1Gi/gp2 thật. Cả hai có ba deployment SUCCEEDED, 15 Terraform attempt rows
READY, job DONE/worker/result, bản ghi Aurora, Redis write/hit/TTL miss/refill
và tải lại cùng kết quả. SQL sau restart đều đạt: 6 loại, 6 Definition,
5 resource, 3 deployment.

Trên AWS, dòng secret tùy chọn hiển thị FAIL vì NOT_CONFIGURED; không được
đổi thành PASS. A01 kiểm binding secret thật. Tất cả kiểm tra bắt buộc của
kịch bản đều đạt theo acceptance scope.

Đường dẫn video (ngoài Git):

- `/tmp/poc-i0406-local-browser-r2/I04-06-captioned.mp4`
- `/tmp/poc-i0407-local-browser-r3/I04-07-captioned.mp4`
- `/tmp/poc-i0408-local-browser-r2/I04-08-captioned.mp4`
- `/home/thanhnt1/.m4/a01-20261015-r4/A01-captioned.mp4`
- `/home/thanhnt1/.m4/e01-20261015-r2/E01-captioned.mp4`
- `/home/thanhnt1/.m4/e02-20261015-r3/E02-captioned.mp4`
- `/home/thanhnt1/.m4/a02-20261016-r2/A02-captioned.mp4`
- `/home/thanhnt1/.m4/a03-20261016-r2/A03-captioned.mp4`

Video đạt đều đã giải mã đầy đủ, kiểm khung hình/phụ đề và bằng chứng secret
scan. A03 video1440×996, Redis/PostgreSQL probe Pod được observer độc lập ghi
đúng năm UID và image/runtime digest đã ghim.

## Kiểm tra code cuối

R15 từ source đóng băng: **1482 Go/race, 0 lỗi, 0 bỏ qua**, PostgreSQL thật
với REQUIRE_POSTGRES=1, Terraform/Redis thật cho integration tương ứng;
Go build/vet đạt. **649 frontend/41 file**, typecheck/lint/test/build đạt.
Docs/PlantUML và diffcheck đạt. Kiểm cuối R16 sau khi khép M04: sáu
bài unittest của checker, Python compilation, REQUIRE_PLANTUML=1 checker
(345 Markdown/297 artifact ID) và git diff --check đều đạt. Hash toàn bộ tracked source và test mới trước/sau R15 khớp;
container/network/file credential của fixture R15 vắng độc lập.

Proof: `.m4/checks-i0412-heartbeat-diagnostics-r15/final-validation.json`.
Không bỏ qua kiểm tra bắt buộc. Writer không chạy được focused PostgreSQL vì
thiếu URL; không tính là pass, Codex đã chạy toàn bộ bằng PostgreSQL thật.
Không lặp A02 chỉ vì bản diagnostic heartbeat sau đó: không đổi threshold,
fencing, success path, provider/module hoặc cấu hình; R15 kiểm lại toàn bộ code.

Bộ kiểm tra tài liệu trước đó bắt buộc một milestone/iteration current, nên
báo lỗi khi M04 đã hoàn tất. Sau khi cập nhật quy tắc chuẩn, writer sửa checker
và CI để chấp nhận 0/0; vẫn chặn nhiều current, thiếu cặp hoặc sai milestone.
R16 chỉ đổi tooling tài liệu; không chạy lại Go/frontend vì nguồn sản phẩm
không đổi sau R15, không bỏ qua kiểm tra bắt buộc của thay đổi này.

Các bản sửa khi chạy thật gồm scoped EKS credentials/target, output subnet
list, nhập boolean, Preview purity, cleanup theo dependency/layer, native
absence với ownership chặt, định dạng lỗi AWS CLI và diagnostic heartbeat.
Không giảm assertion hoặc dùng fake để đạt live scope. Chi tiết và source
commit từng lượt nằm trong biên bản I04-11/12.

A03r1 bị hủy heartbeat trước deadline; PostgreSQL có câu UPDATE heartbeat bị
hủy nhưng không có wait graph để khẳng định blocker/OOM. Manual recovery xác
nhận process dừng, tăng fence/revoke rồi xử lý đúng lock/attempt và cleanup;
original FAIL giữ. R15 bổ sung mã nguyên nhân an toàn, không thay guard.
A03r2 có1032 mẫu SQL đọc được,0heartbeat bị chặn;1 mẫu không lấy được ở thời
điểm đóng fixture (08:48:07.969Z, receipt08:48:08.056Z). Không coi đây là bằng
chứng đã xác định nguyên nhân gốc của lượt trước.

## Dọn sạch và tài nguyên giữ lại

- Mọi marker của các lượt đã chạy đều khép; dirty scan không còn lượt bẩn.
- AWS account452025861381/us-east-1: sáu lớp verifier độc lập đạt. VPC/EKS/
  Aurora/cache/network/EC2/EBS/EIP/ECR/IAM của run đã vắng; không thay account
  settings hoặc xóa tài nguyên người dùng.
- A03 PVC→PV UID→EBS handle được ghi khi Bound; volume chính xác trả
  InvalidVolume.NotFound/DescribeVolumes trước EKS xóa xong. PV là
  awsElasticBlockStore đã migrate sang EBS CSI, được ghi đúng kiểu thực tế.
- Tag index có ARN SG/subnet cũ. Từng ARN có unique CREATED_BY_RUN/aux/run/
  account/region và đúng typed native NotFound; không gọi index rỗng hoặc
  sửa tag để che tài nguyên. Proof `operator-stale-index-proof.json`.
- 28 UID Kubernetes giữ lại và11 service-linked role AWS có sẵn khớp baseline.
- Database/ba role/Vault/private credential mỗi run đều vắng. Ba Docker alias
  riêng mỗi lượt đã dọn; ảnh nguồn giữ. Fixture PostgreSQL chung cuối cùng
  chỉ còn database/role mặc định, đã xác minh label/mount/quyền sở hữu rồi
  xóa đúng container/volume/admin credential; không còn fixture treo.
- Symlink `frontend/node_modules` của người dùng ở runtime checkout giữ nguyên.

Proof chính: `operator-cleanup-closure.json` trong mỗi thư mục run;
A03 có `operator-exact-ebs-absence.json`, `operator-pvc-pv-ebs-link.json`,
`operator-native-layer-closure.json`, `operator-local-cleanup.json`,
`operator-retained-k8s-uids.json`, `operator-all-slr-preservation.json`.
Tổng hợp và fixture: `.m4/m04-final-20261016/summary.json` và
`platform-fixture-cleanup.json`. Không có state/key/credential trong Git.

## Dự toán và chi phí

Giá gốc ngày14-10,13 dòng/cửa sổ5giờ (3giờ chạy +2giờ cleanup):

| Lượt | Dự toán USD |
|---|---:|
| A02 | 2,129994 |
| A03 | 2,050679 |

Đây là dự toán mỗi lượt, không phải trần người dùng đặt hoặc tổng chi phí mọi
lần thử. Cost Explorer đọc lúc08:46Z ngày16-10 cho đúng10 run tag AWS trả
Estimated=true,0group. **Chi phí thực tế chưa xác định được theo run tag;
không coi dữ liệu rỗng là0USD.** Không bật allocation tag hoặc sửa billing.
Proof `.m4/m04-final-20261016/cost-observation.json` và cost-explorer-raw.json;
đầu vào giá/dự toán tại `/tmp/poc-m04-estimate-20261014/`.

## Trạng thái bàn giao

IMP-021 đóng trong phạm vi M04; I04-06..12 hoàn tất, không còn thao tác cloud
hay phiên writer đang chạy. D12/D13/D14, M02/I06-10 và TLS issue#2 vẫn hoãn
ngoài phạm vi này. Bằng chứng lỗi trước giữ nguyên để kiểm toán. Bàn giao theo
quyền người dùng, không yêu cầu duyệt giữa hoặc cuối.
