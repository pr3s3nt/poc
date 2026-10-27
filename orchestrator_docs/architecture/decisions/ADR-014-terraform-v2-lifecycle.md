---
id: ADR-014
artifact: architecture-decision
status: current
last_reviewed: 2026-10-28
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

Người dùng chốt tách tài nguyên mạng và consumer qua Resource References:
VPC của Environment/generation tạo và sở hữu bộ subnet private dùng chung.
Aurora/ElastiCache nhận IDs của bộ subnet này qua outputs VPC, không tự tạo
subnet theo dải CIDR cố định cho mỗi database/cache. Xóa một consumer giữ mạng;
VPC chỉ được destroy khi consumer đã được kiểm và xóa theo ownership hiện hành.
Đợt này vẫn dùng VPC chung mỗi Environment; không mở rộng sang nhiều VPC/peering.

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

Người dùng chốt public API EKS chỉ cho phép danh sách IP cụ thể. PE/Admin cấu
hình danh sách trong Resource Definition EKS, truyền vào Terraform input
`public_access_cidrs`; không thuộc form Workload của Developer. Với EKS IPv4
trong đợt này, mỗi IP dùng CIDR `/32`. Không mặc định mở `0.0.0.0/0` hoặc tự thêm
IP. Giữ private endpoint cho các node trong VPC và IAM/RBAC cho mọi truy cập.
Preflight từng run kiểm IP ra Internet của executor có trong danh sách; thiếu
hoặc không xác định được IP thì chặn và báo để PE/Admin cập nhật cấu hình.

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
theo quyết định2026-10-15: tìm nguyên nhân và sửa/check trước mỗi lượt, không
giới hạn một lần hoặc dừng chỉ vì retry fail; cleanup lần cũ trước retry.
Không thay policy retry executor/unknown outcome ở trên. Cleanup chưa sạch thì
chưa hoàn thành và không bắt đầu run mới.

## Implementation boundary

Kiểm chứng namespace2026-10-15: Kubernetes có thể tạo ServiceAccount default
không có managedFields; không suy thiếu manager thành ownership. Nếu dùng
baseline bổ sung, executor chỉ quan sát read-only ngay sau namespace được
tạo/Ready, trước dependent resources/workload delivery, và lưu namespace UID,
default SA UID cùng metadata an toàn trong artifact mã hóa bất biến theo
instance. Khi destroy, baseline UID phải khớp namespace/SA thực tế và SA
không có custom metadata, owner, secret refs, imagePullSecrets hoặc override
automount. Baseline thiếu/hỏng/UID đổi không đủ quyền xóa. Data source quan sát
nếu có không phải managed destroy identity; refresh không được thay baseline
ban đầu. Không PATCH/adopt default SA hoặc import state để lấy ownership.
Namespace đã vắng chỉ được khép cleanup sau query NotFound/physical IDs gốc;
không coi lỗi đọc/API bất kỳ là absence. Không mở rộng D12–D14.

Quyết định người dùng 2026-10-14: chặng 1 đã được duyệt; bỏ điểm duyệt 2.
Sau I04-11 đạt A01/E01/E02, video và dự toán AWS đã lưu, k8s-4f sạch,
Codex tự chạy I04-12 AWS A02 rồi A03, cleanup ngay và xác minh từng run.
Quyết định2026-10-15 bỏ điểm duyệt cuối: thực hiện đến hết I04-12 và gửi báo
cáo, không chờ phê duyệt; không tự tuyên bố kết quả đã được người dùng duyệt.
Claude cloud viết code/test trên nhánh riêng public; Codex review/checks/merge
và chạy thật local, không gửi secrets vào cloud. Quyết định mới nhất2026-10-14:
cloud hết hạn mức thì chuyển Claude local qua tmux/`clauded`; local tiếp tục
hết thì agent Codex `gpt-6-luna` được viết code/test theo quyền người dùng.
Codex điều phối giữ docs/review/checks/merge, không đổi scope/nghiệm thu/cleanup.

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

## Quyền PostgreSQL cho Terraform state — người dùng chọn A

Người dùng chốt ba DB role riêng: owner khởi tạo schema/cấp quyền, Console
cho runtime quản lý/query thông thường, State cho backend state nội bộ. Cùng
một database; Console không đọc state blob hoặc token hash/capability secrets.
Runtime dùng kết nối/credentials riêng có quyền tối thiểu, không dùng owner
làm tài khoản chạy bình thường. Credentials giữ trong file riêng hạn chế quyền,
không vào database, argv, logs hoặc API. Key mã hóa độc lập với các credentials.
Không dùng SECURITY DEFINER làm đường truy cập state trong thiết kế này.
Chi tiết quyền thuộc state contract và schema; cấu hình role chỉ trên database
mới được giao, không tự reset/regrant database giữ lại.

## TLS deferred — quyết định người dùng

TLS cho PostgreSQL và Terraform state HTTP backend được hoãn theo
[issue #2](https://github.com/pr3s3nt/poc/issues/2) để ưu tiên M04 chạy được.
Hiện không bắt buộc TLS và không áp giới hạn triển khai thay thế (DB cùng máy
hoặc chỉ được dùng loopback). Có thể giữ listener loopback hiện tại nếu đáp
ứng executor; không cần đổi code chỉ vì quyết định này. Ba DB roles, encryption
at rest, capabilities/locks/fencing và bí mật vẫn theo contract hiện hành.


## Amendment — direct Git module authoring, 2026-10-20 (issue #7)

Người dùng chốt PE/Admin được nhập Git HTTPS repository trực tiếp, không cần
Admin duyệt từng repository/module. Thay thế yêu cầu source allowlist cho luồng
này; packaged và historical allowlisted pins tiếp tục hoạt động. Inspection
resolve ref thành full commit, tính digest và giữ verified content trước Save.
Giữ immutable execution/transitive pins, provider release trust, hardened HTTPS
fetch, strict content inspection, private cache và original cleanup artifacts.
Safe origin URL lưu trong bundle contract JSON; không lưu credential. Private
Git auth/SSH không được thêm trong đợt này. UC-03 GIT-01..06 sở hữu UX behavior.

## Git rendering amendment — 2026-10-24

[ADR-021](ADR-021-git-render-bundles.md) supersedes installed-only/Go-owned
rendering policy and exact-three-workload-variable restrictions for Git bundles.
Legacy installed pins retain their contract. Terraform provisioning, dependency
matching, target/state/ownership and cleanup rules in this ADR are unchanged.
Git behavior, inputs, cache and versioning follow the linked contract; this
amendment does not claim implementation or authorize retained-data migration.

ZIP source amendment (2026-10-28): [ADR-025](ADR-025-zip-source-upload.md)
adds uploaded archives to preparation; the verified module contract, providers,
execution identity and state/cleanup lifecycle stay in effect.
