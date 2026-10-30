---
id: UC-04-SPEC
artifact: use-case-specification
status: current
last_reviewed: 2026-10-28
---

# UC-04 — Infrastructure Driver Accounts and Secret Stores

The current replacement design is the infrastructure account catalog AC-01..12
with the V04–V06 execution binding clarification below. Earlier profile/implicit
Environment Connection passages are staged historical behavior, superseded by
ADR-016/017 at their replacement gates. Account registration/verification remains
distinct from resource provisioning and never grants external mutation rights.

## Mục tiêu

Cho phép Platform Engineer/Admin đăng ký và xác minh Connection để Orchestrator:

- Triển khai workload vào Kubernetes cluster đã tồn tại với Execution Profile `internal-k8s`.
- Sử dụng danh tính AWS để tạo và quản lý hạ tầng với Execution Profile `aws-eks`.

Người dùng cấu hình Connection qua Web Console, không cần cấu hình credential
thủ công trên máy chạy backend đối với luồng đăng ký mới.

## Primary actor

- Platform Engineer.
- Admin có quyền quản lý Connection trong Organization.

## Supporting actors

- Kubernetes API.
- AWS Cloud.
- Secret Store.

## Tiền điều kiện

- **PRE-01:** Organization đã tồn tại.
- **PRE-02:** Người dùng đã đăng nhập và có quyền quản lý Connection trong Organization.
- **PRE-03:** Với Kubernetes, cluster đã tồn tại và người dùng có kubeconfig phù hợp.
- **PRE-04:** Với AWS, người dùng có access key và region được phép sử dụng;
  platform đã cấu hình HTTP state backend và durable credential store.
- **PRE-05:** Secret Store sẵn sàng để lưu credential của Connection.

## Trigger

**TRG-01:** Người dùng yêu cầu đăng ký Kubernetes Connection hoặc AWS Connection.

## Main success scenario

1. **MS-01:** Người dùng mở trang `Platform / Connections` và chọn loại Connection cần đăng ký.
2. **MS-02:** Người dùng nhập tên và cung cấp thông tin theo VAR-01 hoặc VAR-02.
3. **MS-03:** Orchestrator kiểm tra dữ liệu và hiển thị thông tin đích kết nối để người dùng kiểm tra.
4. **MS-04:** Người dùng yêu cầu **Kiểm tra và lưu**.
5. **MS-05:** Orchestrator xác minh danh tính, khả năng kết nối và quyền cần thiết theo loại Connection.
6. **MS-06:** Orchestrator lưu credential qua Secret Store và nhận secret reference.
7. **MS-07:** Orchestrator lưu Connection thuộc Organization với trạng thái `READY`.
8. **MS-08:** Orchestrator thông báo thành công và hiển thị Connection trong danh sách.
9. **MS-09:** Connection sẵn sàng để Developer chọn trong Environment Settings
   theo UC-01 hoặc Resource Definition tham chiếu theo UC-03.

## Luồng biến thể

### VAR-01 — Kubernetes Connection / `internal-k8s`

1. Người dùng nhập tên kết nối và tải file hoặc dán nội dung kubeconfig.
2. Orchestrator đọc kubeconfig và liệt kê các context:
   - Nếu có một context hợp lệ, tự chọn context đó.
   - Nếu có nhiều context, yêu cầu người dùng chọn.
3. Orchestrator lấy cluster, endpoint và thông tin xác thực từ context đã chọn.
4. Orchestrator hiển thị context, cluster và endpoint để người dùng kiểm tra. Người dùng không phải nhập Connection ID, Cluster ID hoặc endpoint.
5. Khi xác minh, Orchestrator kiểm tra Kubernetes API truy cập được và quyền tạo:
   - Namespace.
   - Deployment, StatefulSet, Service và Secret trên toàn bộ namespace.
6. Sau khi xác minh thành công, Orchestrator lưu thông tin xác thực cần thiết cho context đã chọn qua Secret Store.
7. Khi triển khai workload, Orchestrator sử dụng credential của Connection này, không phụ thuộc kube context trên máy backend.

### VAR-02 — AWS Connection / `aws-eks`

1. PE/Admin nhập tên kết nối, access key ID, secret access key trong các ô
   được che trên Web Console và allowed regions. Không nhập backend Terraform.
2. Orchestrator xác minh danh tính và xác định AWS account của credential được cung cấp.
3. Orchestrator xác minh regions và quyền cần thiết cho VPC/EKS/ElastiCache/
   Aurora mà không tạo hạ tầng. State backend do platform cung cấp, không
   được chọn theo Connection.
4. Orchestrator lưu credential và secret reference theo [ADR-009 — AWS access-key credential storage](../../architecture/decisions/ADR-009-aws-access-key-storage.md).
5. Khi provision hạ tầng, Orchestrator sử dụng danh tính của AWS Connection được tham chiếu.

AWS Connection cung cấp danh tính cho provisioning hạ tầng; việc xác minh truy
cập vào một EKS cluster không thay thế bước xác minh quyền provisioning.

## Luồng lỗi

- **ERR-01 — Dữ liệu không hợp lệ:** thông báo trường hoặc nội dung cần sửa; không tạo Connection `READY`.
- **ERR-02 — Kubeconfig không có context hợp lệ:** yêu cầu người dùng cung cấp kubeconfig phù hợp.
- **ERR-03 — Thiếu lựa chọn context:** nếu có nhiều context, yêu cầu chọn trước khi xác minh.
- **ERR-04 — Phương thức xác thực chưa hỗ trợ:** giải thích kubeconfig phụ thuộc file bên ngoài hoặc công cụ đăng nhập; không thực thi lệnh trong kubeconfig.
- **ERR-05 — Không thể kết nối hoặc xác thực:** thông báo lỗi an toàn; không lưu Connection `READY`.
- **ERR-06 — Thiếu quyền:** thông báo quyền còn thiếu; không lưu Connection `READY`.
- **ERR-07 — Không thể lưu credential:** không tạo Connection `READY`; cho phép người dùng thử lại.
- **ERR-08 — Không thể lưu Connection sau khi lưu credential:** dọn credential của lần đăng ký thất bại. Nếu cleanup thất bại, ghi nhận lỗi vận hành bằng reference an toàn, không chứa credential.
- **ERR-09 — Không có quyền quản lý Connection:** từ chối thao tác.

Đăng ký thất bại không được ghi đè Connection hoặc credential của một Connection
đã tồn tại.

## Hậu điều kiện

- **POST-01:** Đăng ký thành công tạo Connection `READY` thuộc Organization của người dùng.
- **POST-02:** Metadata và secret reference được lưu; credential không nằm trong domain/database record của Connection.
- **POST-03:** Kubernetes Connection mới có thể được sử dụng mà không cần cấu hình kubeconfig trên máy backend.
- **POST-04:** AWS Connection READY cung cấp scoped credential reference và
  metadata account/principal/region cho Terraform execution.
- **POST-05:** Resource Definition có thể tham chiếu Connection phù hợp.
- **POST-06:** Đăng ký Connection không tạo workload, VPC, EKS hoặc Aurora.

## Quy tắc nghiệp vụ

- **BR-01:** Connection thuộc một Organization và có ID duy nhất trong Organization. Hệ thống tạo ID từ tên kết nối và xử lý xung đột để không ghi đè Connection hiện có.
- **BR-02:** Credential được lưu qua Secret Store. Domain/database record của Connection chỉ giữ metadata và secret reference.
- **BR-03:** UC-04 chỉ đăng ký và xác minh Connection; provisioning thuộc UC-06 và UC-08.
- **BR-04:** Chỉ Connection READY mới được chọn trong Environment Settings hoặc triển khai.
- **BR-05:** Luồng đăng ký Kubernetes mới nhận kubeconfig qua upload hoặc nội dung được dán; không yêu cầu context có sẵn trên host backend.
- **BR-06:** Hỗ trợ token nhúng hoặc chứng chỉ/private key nhúng, cùng thông tin cluster và cấu hình xác minh TLS cần thiết.
- **BR-07:** Không hỗ trợ credential tham chiếu file bên ngoài hoặc xác thực qua `exec`. Hệ thống không thực thi lệnh từ kubeconfig tải lên.
- **BR-08:** Xác minh Kubernetes kiểm tra API và các quyền trong VAR-01 mà không tạo resource trên cluster.
- **BR-09:** Credential không được xuất hiện trong API đọc, log, thông báo lỗi, snapshot, Preview, manifest hoặc Git. Nội dung kubeconfig không được trả lại qua read model.
- **BR-10:** Xác minh và thực thi chỉ được lấy credential đúng Organization/Connection. Connection mới không fallback sang credential mặc định trên host.
- **BR-11:** Credential AWS dùng cho provisioning không được cấp cho workload. Luồng lưu trữ và sử dụng tuân theo ADR-009.
- **BR-12:** AWS READY cần xác minh identity, region và provisioning
  capabilities; STS success một mình không đủ. Registration không tạo hạ tầng;
  execution preflight kiểm lại quyền. State backend là trách nhiệm platform.
- **BR-13:** Trạng thái `READY` phản ánh kết quả xác minh tại thời điểm đăng ký; không cam kết giám sát kết nối liên tục.
- **BR-14:** Loại Connection và phương thức xác thực phải được phân biệt để có thể bổ sung phương thức xác thực AWS sau này mà không thay đổi ý nghĩa của Connection hoặc tham chiếu từ Resource Definition.
- **BR-15:** Kubernetes Connection chỉ dùng `KUBECONFIG` với credential riêng đúng Organization/Connection. Từ chối body cũ `{key,clusterId,kubeContext}`, `HOST_CONTEXT`, thiếu authentication type hoặc thiếu credential; không inference, chuyển đổi hay fallback sang host. Seed thiếu credential không được READY; default marker có thể tham chiếu record VERIFYING nhưng không làm Environment tự cấu hình.
- **BR-16:** Đăng ký Connection mới không tự động thay đổi connection mặc định của Organization.

## Giao diện

Trang `Platform / Connections` gồm:

- Danh sách Connection `READY` thuộc Organization, hiển thị tên/ID, loại, cluster/endpoint và trạng thái phù hợp với từng loại.
- Form đăng ký Kubernetes với tên kết nối và lựa chọn tải file hoặc dán kubeconfig.
- Danh sách context khi kubeconfig có nhiều context.
- Thông tin context, cluster và endpoint được chọn.
- Nút **Kiểm tra và lưu**.
- Trạng thái đang xử lý, thông báo thành công và lỗi có hướng dẫn xử lý.

Form Kubernetes không yêu cầu nhập Connection ID hoặc Cluster ID. Form AWS
có name, các ô access key được che, allowed regions và Kiểm tra và lưu.
Read/list chỉ có metadata; key không được đọc lại qua Console.

## Ngoài phạm vi

- **OOS-01:** AWS authentication ngoài access key onboarding đã chốt;
  không dùng host credential cho product execution. Access key/Vault flow
  thuộc VAR-02, BR-10/11/12.
- **OOS-02:** Tạo cluster Kubernetes từ form đăng ký.
- **OOS-03:** Xác thực Kubernetes qua `exec` hoặc file credential bên ngoài.
- **OOS-04:** Xóa tài khoản/Connection; V01 bổ sung sửa theo AC-01..12 bên dưới. V02–V03 không yêu cầu migration dữ liệu cũ theo quyết định 2026-10-17.
- **OOS-05:** Tự động rotation, health monitoring liên tục và audit history; thay credential tường minh cùng đích thuộc AC-07.
- **OOS-06:** Humanitec-style Agent, private-cluster tunnel, GitOps mode và cài đặt Operator tự động.

## Ranh giới tài liệu

Trạng thái triển khai thực tế được theo dõi tại [CURRENT_STATE.md](../../CURRENT_STATE.md).
Specification định nghĩa yêu cầu; realization/contracts định nghĩa collaboration.
Kết quả triển khai và verification thuộc CURRENT_STATE/deviations.

## Secret Store Connection registration (current feature scope)

- **SS-01:** Platform Engineer/Admin mở Platform → Secret stores, nhập name,
  Vault backend/workload addresses, KV v2 mount, Kubernetes auth mount, TLS
  settings và token trong concealed field; generated key, không phải user-supplied ID.
- **SS-02:** Backend role/org gates, strict bounded validation, verify KV v2
  capabilities bằng probe path của attempt; cleanup probe trước READY.
- **SS-03:** Token lưu trong platform credential store riêng; insert metadata
  scoped READY store; persistence failure cleanup attempt credential như UC-04.
- **SS-04:** List/detail và Developer choices chỉ safe metadata; không token,
  credential ref, raw Vault error hoặc secret-read API. Store đăng ký không tự chọn.
- **SS-05:** Hai Vault stores có thể khác endpoint/mount/auth; addresses identity
  không được sửa dưới existing key. Workload auth phải hợp lệ trước Deploy.
- **SS-06:** First provider VAULT_KV_V2. Platform/backend credentials không thuộc
  selected workload secret store. Missing configuration fails closed; no fallback.

- **SS-07:** Local Compose tự đăng ký Vault đi kèm như một workload store thông
  thường, key ổn định `platform-vault`, name `Platform Vault`, không có legacy flag.
  Bootstrap chạy cùng validation/verifier và platform credential persistence với
  registration; READY chỉ sau probe/cleanup thành công. Token file chỉ là input
  của bootstrap, không phải cơ chế runtime riêng của store. Restart idempotent,
  không tạo store/credential dư thừa, không tự chọn cho Environment mới.
- **SS-08:** Restart chỉ giữ hoặc refresh kho ordinary do bootstrap quản lý
  (`verification.bootstrap=compose`). CAS kiểm ID, credential ref và endpoint
  identity; không overwrite kho người dùng. Verify/probe-cleanup trước READY;
  failure dọn credential của attempt, không dọn credential của winner.
  Không convert/backfill kho cũ, không dùng legacy Vault flags hoặc token global
  khi runtime. Token chỉ resolve từ private scoped credential ref.

Supporting flow/design: [ADR-012](../../architecture/decisions/ADR-012-environment-stores-and-transitions.md).

## Implicit internal cluster binding

- **BR-17:** A READY Kubernetes Connection selected by an Environment is sufficient to bind its implicit existing-cluster node; no additional per-cluster Definition registration is required. Selection/registration does not provision workloads or cloud infrastructure.

See [ADR-013](../../architecture/decisions/ADR-013-implicit-existing-cluster.md).


## EKS target

- **V2-04:** EKS target do provision trả có identity đúng Organization/Connection
  đủ tái tạo credential cho invocation sau restart. Không persist kubeconfig/
  đường dẫn tạm, không dùng host context. Existing k8s-4f dùng KUBECONFIG
  theo BR-15; HTTP state token tách khỏi AWS credential.


## V01 — Danh mục tài khoản hạ tầng (AC-01..12)

[ADR-015](../../architecture/decisions/ADR-015-infrastructure-driver-accounts.md)
quy định mô hình và ranh giới rollout. Các quy tắc sau mở rộng UC-04; luồng
Connection cũ giữ nguyên cho consumers đến V02–V03, không coi là mô hình đích.

- **AC-01:** PE/Admin quản lý tài khoản thuộc Organization của session; không
  nhận Organization do client chọn. Developer không có quyền quản lý hoặc đọc
  credential. Không có phạm vi áp dụng hoặc gán tài khoản mới cho môi trường.
- **AC-02:** Tài khoản có ID/key ổn định, tên (1–100 Unicode, không control),
  `type` AWS/INTERNAL, authenticationType AWS_ACCESS_KEY hoặc AWS_EC2_ROLE cho AWS,
  KUBECONFIG cho Internal,
  config/verification không bí mật, private ref, status và version >= 1.
  Nhiều tài khoản AWS/Internal được phép cùng tồn tại.
- **AC-03:** Internal dùng upload/paste kubeconfig, inspect/chọn context và verify
  như VAR-01. Descriptor an toàn chứa cluster/context/endpoint/TLS metadata;
  auth reference chỉ cho nội bộ. Cluster có sẵn không thuộc quyền tạo/xóa của run.
- **AC-04:** AWS dùng access key theo VAR-02/ADR-009. `verificationRegions` là
  vùng cần kiểm tra lúc đăng ký/xác minh, không phải phạm vi áp dụng tài khoản.
  Region khi provision do Driver Inputs chọn ở V02; V01 chưa đổi execution cũ.
- **AC-05:** Danh sách/chi tiết trả metadata an toàn và version, không secretRef/
  raw verification/key/kubeconfig. Danh mục sẵn sàng cho PE/driver consumer tương
  lai; V01 không thêm chọn Driver Account tại Definition hoặc thay planner.
- **AC-06:** Sửa tên yêu cầu version hiện tại; giữ nguyên ID/key/type/auth/target/
  ref/verification. Lỗi stale báo 409 và yêu cầu tải lại; không tự ghi đè hoặc retry
  mutation. Lưu thành công tăng version một lần; load lại/restart thấy đúng tên.
- **AC-07:** Thay credential là thao tác riêng được che, xác minh trước lưu.
  Không prefill credential. Cùng account phải giữ AWS account ID hoặc cùng
  Internal target/context/endpoint/trust; khác đích yêu cầu đăng ký tài khoản mới.
  CAS mới công bố ref/verification/version; thất bại giữ nguyên bản đang dùng.
  AWS key mới phải xác minh mọi vùng legacy còn được phép sử dụng trước khi
  công bố; không đổi ngầm vùng của môi trường đang liên kết.
- **AC-08:** “Kiểm tra kết nối” dùng credential lưu riêng và network chỉ đọc;
  không tạo workload/VPC/EKS/database/cache. Kiểm tra quyền/API/AWS theo các
  bảo đảm hiện có; sai loại, thiếu quyền, hết hạn, đích unreachable báo lỗi an toàn.
  Success tăng version và refresh metadata; lỗi giữ record cũ nhưng hiển thị
  kiểm tra vừa thất bại. READY chỉ phản ánh lần xác minh thành công gần nhất.
- **AC-09:** Mọi cập nhật dùng CAS. Request bị hủy hoặc race không thể overwrite
  winner. Cleanup chỉ xóa object của attempt thất bại; credential cũ đã commit
  được giữ để không phá persisted targets. Lỗi cleanup phải có dấu vận hành an
  toàn và không được xem là success. Commit chưa rõ kết quả phải giữ object
  có thể đã commit, báo cần GET để xác định kết quả; không tự retry mutation.
  Không có API đọc/xóa credential công khai.
- **AC-10:** UI “Tài khoản hạ tầng” có tạo/xem/sửa tên, thay credential tường minh,
  kiểm tra kết nối và tải lại; phân biệt loại hạ tầng với auth, vùng kiểm tra với
  deployment region. Loading/empty/error/stale/saving/success và reload lỗi tách
  rõ; đổi draft vô hiệu response cũ, không tự submit/replay mutation. Khi tạo
  hoặc sửa chưa rõ kết quả, chặn gửi lại đến khi GET thành công và người dùng
  đã xem kết quả; GET lỗi tiếp tục chặn và cho tải lại.
- **AC-11:** Dữ liệu và scoped credential còn đúng sau khởi động lại; dùng Vault
  durable với SQL thật khi kiểm chứng persistence. Không ghi secret vào state,
  DTO, logs, errors, graph, video hoặc Git. Memory chỉ hỗ trợ fake/no-persistent
  tests như boundary hiện có.
- **AC-12:** V01 giữ đường triển khai cũ; không tự chuyển dữ liệu hay xóa tài nguyên.
  Theo quyết định người dùng 2026-10-17, V02–V03 không cần migration dữ liệu cũ.
  Baseline/state không tương thích phải fail rõ, không tự upgrade/reset retained
  state. Chỉ bỏ đường cũ sau khi đường thay thế được kiểm chứng.


## Quyền reset dữ liệu phát triển — bổ sung 2026-10-17

Người dùng yêu cầu xóa dữ liệu sản phẩm cũ vì chưa có người dùng thực tế.
V01 được phép reset database/state phát triển cũ của dự án sau khi xác định rõ
instance/namespace/ownership và có baseline mới để khởi tạo. Không cần migration
hay giữ tương thích dữ liệu cũ. Đây là quyền reset tường minh của task, không phải
cơ chế tự reset khi startup hoặc quyền xóa tài nguyên không thuộc dữ liệu sản phẩm.
Codex thực hiện và ghi evidence phạm vi/kết quả; writer không truy cập retained
state hoặc tự chạy thao tác hạ tầng. Các yêu cầu secret scope, CAS, pinned targets
trên baseline mới và bảo vệ tài nguyên ngoài task vẫn áp dụng.

## V04–V06 execution binding clarification

[ADR-017](../../architecture/decisions/ADR-017-definition-dependency-execution.md)
replaces profile/Environment-selected infrastructure with normal Definition
matching/references. Every Terraform resource uses its own winning Definition's
READY Organization Driver Account and literal region. Internal targets bind the
referenced account; EKS target identity pins the producing descriptor, original
account/version and region for fresh invocation authentication. Registration and
verification alone create no infrastructure and do not imply runtime readiness.

V06.08 retires executionProfile and Environment deployment binding/transition
consumers; it preserves AC-01..12 account management, credential version/CAS,
immutable committed refs and Secret Store capabilities. Legacy allowed/default
region constraints do not survive as account applicability policy. Different
resource accounts/regions require valid actual contracts/network references,
not equality across an Environment. Never fall back to Environment/host identity.
Historical Connection storage identity may continue backing Driver Accounts;
that name is not evidence of an obsolete execution path. Incompatible old data
fails safely; no startup reset or implicit conversion is introduced.


Final V06.08 removes the unused Organization deployment-default choice marker
with the retired selector. Registration continues to preserve the whole
Organization identity/metadata and never configures an Environment or changes
Definition selection. Organization-scoped account ownership, verification,
credential isolation and role gates remain unchanged; BR-15/16 default-marker
wording above is historical after that removal.

## EC2 role accounts — issue #6

[ADR-018](../../architecture/decisions/ADR-018-ec2-role-driver-accounts.md) extends
AC-02/04/07/08 with AWS_EC2_ROLE, alongside AWS_ACCESS_KEY.

- **EC2-01:** PE/Admin explicitly registers name, canonical expected Role ARN and
  1–10 verificationRegions in the session Organization. No credential input.
- **EC2-02:** Verify IMDSv2 temporary credentials, STS identity and IAM role
  ARN/unique RoleId, regional read probes and existing capability simulation
  before READY. Wrong role/account, expired credentials, missing permission or
  metadata failure saves nothing; no host credential fallback.
- **EC2-03:** Store roleArn/roleId/accountId and safe verification only; secretRef
  is empty. Reverify preserves both pinned role identifiers and uses version CAS.
  Rename follows AC-06. Reject credential replacement or authentication changes.
- **EC2-04:** Terraform and EKS consumers explicitly select this scoped READY
  account through existing Definition bindings. Refresh credentials during long
  Terraform runs while preserving role identity, including destroy/cleanup.
- **EC2-05:** UI distinguishes EC2 role from access key, requests expected Role
  ARN and regions, hides access-key/replacement controls for role accounts and
  preserves AC-10 error/stale/unknown-outcome recovery behavior.

## Account diagnostics — DIAG-01..14

The user authorized implementation of the account-integrated diagnostic mock on
2026-10-25. This extension adds on-demand diagnostic history to UC-04; it does
not introduce global auditing or continuous monitoring from historical OOS-05.

- **DIAG-01:** Platform Engineer/Admin starts, reads and cancels diagnostics only
  for an Infrastructure Account in the session Organization. Developer has no
  management access; foreign or unknown account/report/context is not disclosed.
- **DIAG-02:** The account list shows a scoped/time-stamped latest diagnostic
  summary. Account detail owns Information and Diagnose tabs. Existing account
  registration, rename, verify, credential replacement and version guards remain.
- **DIAG-03:** Connection verification and diagnostic readiness are independent.
  A diagnostic run never changes Connection status/version, credentials, an
  Environment binding, Application draft/current state or Deployment Set.
- **DIAG-04:** GENERAL works without Application/Environment and is read-only.
  USE_CASE selects supported capabilities, explicit AWS region where relevant,
  optional Application/Environment and CONDITIONS or supported EXECUTION mode.
  Selecting diagnostic context does not change account applicability or policy.
- **DIAG-05:** Probes use the saved scoped account credential and platform runtime.
  TLS trust/expiry/hostname, reachability, authentication and authorization have
  distinct safe findings when supported by observed evidence. Host fallback and
  fixture success in production are forbidden.
- **DIAG-06:** Required checks derive from the selected work and actual known
  configuration/Definitions. No Application means Application-specific matching,
  images, secrets and route requirements remain UNKNOWN. Optional components
  are required only when the selected deployment path needs them.
- **DIAG-07:** PASS requires actual evidence; FAIL is an observed unmet applicable
  condition; BLOCKED names a prerequisite; UNKNOWN means insufficient evidence;
  NA explains genuine non-applicability. Missing permission and unsupported
  execution never count as PASS. Independent checks may continue after failure.
- **DIAG-08:** Each finding exposes a safe observation, expected condition,
  affected use cases, remediation and dependency links. VI/EN catalogs map
  stable codes. No credential/ref, raw error, kubeconfig, secret value, PEM,
  Terraform state or arbitrary provider body enters report, log or export.
- **DIAG-09:** Backend-derived progress and durable Organization/account-scoped
  reports/history survive reload and process restart. The immutable scope pins
  account/config versions, purpose, mode, region, selections and context/time.
  Changes make earlier evidence historical/stale, not present readiness.
- **DIAG-10:** Runs support explicit stop/cancel and honest partial/interrupted
  state. A bounded owner/lease prevents concurrent active account probes and
  stale-worker writes. Cancel/retry/reload never replay an unknown start outcome.
- **DIAG-11:** CONDITIONS performs no external mutation. Kubernetes EXECUTION
  creates only bounded unique run-owned probe resources in a new namespace,
  validates ownership/UID before cleanup and verifies absence. A failed or
  unconfirmed cleanup is visible and prevents an all-ready claim.
- **DIAG-12:** Diagnostics never apply to existing Application namespaces or
  implicitly create/destroy paid cloud infrastructure. Unsupported provider
  execution proof is UNKNOWN with its limit visible; IAM simulation is identified
  as policy evidence. AWS identity success alone does not prove provisioning.
- **DIAG-13:** Filters, details, history and export retain scope and truthful
  counts. Account/region/context switches discard stale UI responses. Loading,
  empty, report-load failure, validation, in-flight, cancellation and partial
  results remain distinct. Export uses safe backend report facts.
- **DIAG-14:** Acceptance exercises real user flows through Console and compares
  findings with actual owned runtime facts, including repair/rerun and durable
  history. Fake UI/adapters cannot prove real account readiness. Merge and
  post-merge checks follow the user's requested Claude/Codex workflow.

[Protocol and probe semantics](../../architecture/contracts/account-diagnostics.md)
are shared; [delivery and acceptance](../../implementation/account-diagnostics.md)
route the implementation without replacing these requirements.

## Staging bootstrap — quyết định 2026-10-26

Startup mặc định dùng catalog `staging`: tạo Organization, các tài khoản đăng
nhập theo profile và chỉ năm Resource Types `k8s-cluster`, `k8s-namespace`,
`postgres`, `redis`, `workload` còn thiếu. Không seed Driver Accounts,
Applications, Environments, Definitions hoặc Rendering templates. Không ghi đè
schema đã có; không xóa dữ liệu cũ khi restart. Fixture legacy/terraform-k8s
và acceptance-v2 tiếp tục là lựa chọn tường minh cho development/tests.

Console chỉ cung cấp Driver Accounts; bỏ màn hình/menu Connections cũ. Backing
record và credential store nội bộ không đổi. Người vận hành đăng ký Internal
Driver Account `k8s-4f` bằng kubeconfig thật; startup không tạo account READY giả.

## CD provider bindings — ADR-027

- **CD-01:** PE/Admin can register/version/verify an Organization-scoped delivery
  binding. Management identity, Git writer identity, controller read identity and
  actual workload target are distinct. Verification checks target registration
  and capabilities read-only; it neither installs Fleet nor enrolls a cluster.
- **CD-02:** Verification is not deployment success. Missing/unready/foreign or
  incompatible bindings fail before delivery side effects; no host credentials,
  wildcard target or default controller namespace is inferred. Original versions
  remain usable for observation/cleanup while their credentials remain valid.
- **CD-03:** Credential inputs are write-only and use private storage. Developers
  see eligible names/capabilities but cannot alter credentials or controller
  topology. No Git credential, raw provider config or secret enters review/logs.

- **CD-04 (user decision 2026-10-29):** PE/Admin manage CD bindings through
  **Cấu hình CD** at `/ui/platform/cd-providers`: list, create, view, create a new
  immutable configuration version, verify and inspect Environment usage. Ordinary
  forms are sufficient; no JSON/Score editor is required for this workflow.
- **CD-05:** Fleet bindings explicitly choose **MANAGED** (Orchestrator creates
  scoped GitRepos) or **EXISTING** (use a named preexisting GitRepo). Existing
  registration/controller/reader credentials remain platform-owned. Verification
  discovers and pins its UID, repository, branch, root, target and agent namespace;
  ambiguous/multiple-target or overlapping publication scope is rejected. Changes
  to existing configuration invalidate verification. Orchestrator must never
  replace/delete the preexisting GitRepo or publish outside its assigned subtree.
- **CD-06:** Controller namespace and agent namespace are separate configuration
  identities. Environment Settings links to this platform page when no provider
  is ready; it selects a verified version, never edits controller/Git credentials.
- **CD-07:** Browser acceptance performs happy paths through ordinary forms for
  CD setup, Environment selection, application/workload/dependency configuration,
  Preview/Deploy/update/manager transfer/removal. No pasted JSON, Score/YAML or
  mutation API shortcuts substitute for UI actions. Technical read-only assertions
  may verify real Git revisions, resource ownership, data, readiness and cleanup.
