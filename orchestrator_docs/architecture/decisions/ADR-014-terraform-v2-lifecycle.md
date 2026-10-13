---
id: ADR-014
artifact: architecture-decision
status: current
last_reviewed: 2026-10-13
related: [UC-03, UC-04, UC-05, UC-06, UC-07, UC-08, UC-09, UC-16]
---

# ADR-014 — Terraform v2: ownership, modules, state và execution

Status: Accepted — 2026-10-13, theo các quyết định người dùng chốt ở Bước 1.
Đây là thiết kế cần implement, không phải bằng chứng code hoặc acceptance đã đạt.

## Context và supersession

V2 dùng form Workload hiện có, Orchestrator chọn Definition và dependency,
Terraform quản lý hạ tầng, score-k8s sinh manifest ứng dụng, delivery chờ Ready.
Không migration/import/tiếp quản tài nguyên hay dữ liệu thử nghiệm v1. Acceptance
tạo tài nguyên mới; cụm k8s-4f và mọi tài nguyên ngoài run được giữ nguyên.

Trong phạm vi v2, quyết định này thay thế executor Kubernetes cho namespace/
database, giới hạn module nhúng và state tạm ở D01/D03, chính sách renderer
tùy chọn chỉ cho cluster nội bộ trong ADR-010, và account Aurora riêng cùng
tên tài nguyên chứa run ID trong ADR-011. ADR-001 vẫn sở hữu enrichment/scope. Giữ descriptor, trọng số matching,
scoped credentials, admission/CAS/
fencing, generation isolation và separation của ADR-002/003/008/009/012/013.
Không mở rộng migration dữ liệu hoặc recovery của ADR-012 sang Terraform v2.
Lịch sử triển khai và verification cũ được giữ nguyên.

## Ownership và lifecycle

| Resource | Owner | Lifecycle |
|---|---|---|
| k8s-4f, Connections, platform database/Vault có sẵn | Platform | Chỉ tham chiếu; không thuộc quyền destroy của workload hoặc test run |
| VPC, EKS, namespace tạo bởi v2 | Environment + target generation | Tái sử dụng trong Environment/generation đó; không dùng chung xuyên Environment |
| Redis/PostgreSQL private | Workload trong Environment/generation | Identity theo private descriptor; một owner |
| Redis/PostgreSQL shared | Environment/generation | Identity theo shared descriptor; nhiều consumer cùng contract |

Deploy lại dùng identity/state cũ. Remove workload/dependency chỉ gỡ workload
objects và đánh dấu resource không còn được dùng là UNREFERENCED; giữ dữ liệu.
Người dùng đã chốt thao tác **Xóa tài nguyên** trên trang chi tiết tài nguyên
trong Environment. Chỉ PE/Admin được bấm; Developer không có quyền. Tài nguyên
phải UNREFERENCED, không còn consumer và Environment không có operation đang
chạy. Backend kiểm lại các điều kiện trong giao dịch nhận operation.

Hộp xác nhận cảnh báo mất dữ liệu, yêu cầu gõ lại tên tài nguyên và liệt kê
đúng các tài nguyên đã tick. Người dùng chốt chỉ root được tick mặc định;
dependency cùng Environment/generation để trống và được chọn tường minh.
Không hiển thị platform. Khóa mục có consumer ngoài tập đã chọn, nêu lý do;
bỏ tick kéo theo bỏ lựa chọn trở nên không hợp lệ và giải thích. Backend kiểm
lại toàn bộ selected set trong admission, từ chối cả request nếu không hợp lệ
và không tự thêm mục xóa. Chi tiết thuộc UC-07 DEL-BR-01.
Xóa theo thứ tự consumer trước provider, dùng module/target/state gốc. Hiển thị
REQUESTED/CHECKING/DESTROYING/VERIFYING, kết quả kiểm tra tài nguyên đã hết
và ID còn lại nếu lỗi. Executor có contract/tests tạo, cập nhật và xóa.

V2-A01 phải chứng minh xóa tài nguyên qua UI sau khi gỡ các workload/consumer.
Cleanup script độc lập vẫn thu hồi tài nguyên test khi browser/runner lỗi;
script không thay bước xóa qua UI. Chi tiết behavior thuộc UC-07 và UC-09.

Đổi class/module/target không tự chuyển dữ liệu hoặc thay thế âm thầm. Preview
phải chỉ ra thay đổi và yêu cầu xác nhận riêng nếu cần tạo thay thế; tài nguyên
cũ được giữ đến cleanup riêng. Ordinary update không được apply plan có delete/
replace tài nguyên hiện hữu. V2 chặn trường hợp đó trước apply; việc thay thế
chỉ được thực hiện qua operation tạo identity/generation mới được xác nhận.

Cleanup phân biệt CREATED_BY_RUN, PREEXISTING và UPDATED_BY_RUN.
Cleanup tự động của run chỉ được xóa CREATED_BY_RUN; không xóa
PREEXISTING/UPDATED_BY_RUN. Xóa do PE/Admin yêu cầu được kiểm theo ownership
bền vững của resource và phạm vi đã xác nhận, không theo run vừa thực thi.
Cả hai cách đều không xóa resource platform, resource ngoài quyền hoặc có
consumer ngoài tập xóa. Không rollback tự động tài nguyên đã cập nhật. Run ID theo dõi provenance,
không thay thế resource ownership.

## Module, provider và matching

Platform Admin cấu hình allowlist nguồn; PE đăng ký Definition từ nguồn đó.
Module cố định bằng full Git commit + content digest hoặc artifact SHA-256;
không nhận branch/latest, arbitrary command hoặc mutable download. Release
manifest pin Terraform binary, providers/lockfile, renderer binary và module
digests. Bản nâng cấp chọn tường minh và Preview mới; không tự nâng lúc Deploy.

Module contract, input/output mapping và trust boundary nằm tại
[Terraform module contract](../contracts/terraform-modules.md). V2 hỗ trợ
namespace, Redis, PostgreSQL Kubernetes; VPC, EKS, ElastiCache, Aurora AWS.
Echo/External và custom drivers vẫn deferred. Connection tới k8s-4f không phải
Echo/External application resource.

**Quyết định của người dùng:** AWS Connection nhận access key qua các ô được
che trên Web Console. Key lưu bằng Vault reference đúng Organization/Connection
theo UC-04 VAR-02, BR-02/09/10/11/12, V2-04 và OC-05. Product execution
chỉ resolve credential này,
không dùng credential của host. Đây là lựa chọn được giữ nguyên, không phải
một đề xuất hoặc yêu cầu đọc key từ máy chạy backend.

AWS resources của một Environment dùng cùng Connection/account/region đã pin;
không hỗ trợ account/region override cho Aurora hoặc ElastiCache. Kubernetes
provider dùng Environment Connection hoặc EKS target do node EKS tạo. Credential
được resolve riêng cho invocation; Workload không nhận provider credentials.

Giữ matching theo type/profile và năm criteria có trọng số hiện hành; class
`in-cluster` hoặc `cloud-managed` thuộc dependency. Không match/tie thì fail;
không tự đổi class, target hoặc chọn Definition thay thế. Internal cluster
system binding giữ ngoại lệ ADR-013. Workload v2 bắt buộc có score-k8s Definition/
bundle hợp lệ cho cả internal-k8s và aws-eks; không fallback native renderer.

## State, locking và recovery tối thiểu

Chọn Terraform HTTP backend do Orchestrator phục vụ, lưu state mã hóa trong
PostgreSQL platform. Mỗi resource instance có một state, bao gồm toàn bộ
provider objects do module của instance đó quản lý. Identity chứa Organization,
Environment, target generation và logical descriptor/scope; không chứa run ID.
Physical names ổn định theo instance, không thay theo lần Deploy/retry.

Hai lớp khóa: persisted Environment operation và Terraform state lock. Kiểm
owner/fencing trên mọi state write/lock/release; timeout heartbeat không tự mở
khóa hoặc cho executor mới chạy chồng. Secrets trong state khác với metadata
outputs: state là encrypted restricted blob; metadata chỉ giữ secret references.
Chi tiết tại [state contract](../contracts/terraform-state.md).

Lỗi transient có retry hữu hạn sau khi kiểm state/actual result. Input/auth/
permission/unknown outcome không tự retry apply. Gián đoạn ghi nhận stage,
attempt, ownership và giữ khóa cần thiết; vận hành xác nhận process đã dừng
trước manual recovery/cleanup. State thiếu và kết quả chưa rõ thì fail closed.

Người dùng hoãn full recovery vào [D12](../../backlog/D12-terraform-recovery.md),
orphan reconciliation vào [D13](../../backlog/D13-terraform-orphan-reconciliation.md),
platform backup/restore vào [D14](../../backlog/D14-platform-state-backup.md).
Mất platform database/key có thể mất khả năng quản lý hạ tầng; v2 không tuyên bố
disaster recovery. Acceptance cleanup sau runner/browser lỗi vẫn bắt buộc.

## Execution và acceptance

[V2 acceptance runbook](../../operations/terraform-v2-acceptance.md) sở hữu quyền
thực thi, preflight, scenario assertions, retry và cleanup evidence. Dùng app
frontend/backend/worker hiện có, bổ sung Redis; kiểm trọn job flow qua UI ứng dụng
sau khi khai báo/deploy qua chính Web Console. Không nhập Score hoặc gọi API
để thay bước nghiệp vụ. Backend/adapters thật, headed video có phụ đề Việt.

Không có trần chi phí cố định do người dùng đặt; chọn cấu hình nhỏ đủ chạy,
ước tính trước run, chạy tuần tự và xóa ngay sau verification. Scenario retry
tối đa một lần sau sửa lỗi; cleanup lần cũ trước retry. Cleanup chưa sạch thì
chưa hoàn thành và không bắt đầu run mới.

## Implementation boundary

Lộ trình M04 có ba chặng và các điểm dừng người dùng đã yêu cầu: chặng 1
sửa tài liệu/plan và chờ duyệt; chặng 2 code, PostgreSQL thật và k8s-4f cùng
video rồi chờ duyệt video A01 và dự toán AWS; chặng 3 mới chạy AWS, xác minh
cleanup và bàn giao để người dùng duyệt. Không dùng quyền AWS đã giao để
bỏ qua điểm dừng chặng 2.

Code gaps được ghi trong
[deviations](../../implementation/deviations.md); traceability ghi planned tests
và acceptance, không gán evidence cũ cho v2. Bước 1 đủ khi artifacts bên dưới
nhất quán và không còn product/security/external-scope decision mở. Exact
release versions, kích thước nhỏ tương thích và deadline hữu hạn là lựa chọn
implementation/preflight, phải ghi trước execution và tuân contract đã chốt.

## Technical references

- [Terraform HTTP backend](https://developer.hashicorp.com/terraform/language/backend/http)
  cho giao thức state/lock.
- [State locking](https://developer.hashicorp.com/terraform/language/state/locking)
  cho yêu cầu không ghi state đồng thời.
- [Provider dependency lock](https://developer.hashicorp.com/terraform/language/files/dependency-lock)
  cho pin provider; module revision/content cần được pin riêng.
- [Sensitive data](https://developer.hashicorp.com/terraform/language/manage-sensitive-data)
  cho việc state/plan có thể chứa secret dù output được đánh dấu sensitive.
