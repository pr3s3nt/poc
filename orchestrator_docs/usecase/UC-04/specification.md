---
id: UC-04-SPEC
artifact: use-case-specification
status: current
last_reviewed: 2026-10-07
---

# UC-04 — Configure Execution Profile Connections

## Mục tiêu

Cho phép Platform Engineer/Admin đăng ký và xác minh Connection để Orchestrator:

- Triển khai workload vào Kubernetes cluster đã tồn tại với Execution Profile `internal-k8s`.
- Sử dụng danh tính AWS để tạo và quản lý hạ tầng với Execution Profile `aws-eks`.

Người dùng cấu hình Connection qua Web Console, không cần cấu hình credential
thủ công trên máy chạy backend đối với luồng đăng ký mới.

## Phạm vi delivery

- **Giai đoạn đầu:** đăng ký Kubernetes Connection bằng kubeconfig có token hoặc chứng chỉ nhúng.
- **Giai đoạn sau:** đăng ký AWS Connection, xác minh quyền provisioning và sử dụng credential trong Terraform executor.
- AWS Connection được xác định trong specification này để làm cơ sở thiết kế; không phải tính năng được triển khai trong giai đoạn đầu.

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
- **PRE-04:** Với AWS, người dùng có danh tính AWS và thông tin region, Terraform backend cần cấu hình.
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

**Biến thể này được triển khai ở giai đoạn sau.**

1. Người dùng nhập tên kết nối, access key ID, secret access key, allowed regions và Terraform backend configuration.
2. Orchestrator xác minh danh tính và xác định AWS account của credential được cung cấp.
3. Orchestrator kiểm tra allowed regions, Terraform backend và quyền cần thiết để tạo/đọc/quản lý VPC, EKS, Aurora cùng Terraform state.
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
- **POST-04:** AWS Connection hợp lệ cung cấp danh tính và cấu hình cho luồng provisioning ở giai đoạn sau.
- **POST-05:** Resource Definition có thể tham chiếu Connection phù hợp.
- **POST-06:** Đăng ký Connection không tạo workload, VPC, EKS hoặc Aurora.

## Quy tắc nghiệp vụ

- **BR-01:** Connection thuộc một Organization và có ID duy nhất trong Organization. Hệ thống tạo ID từ tên kết nối và xử lý xung đột để không ghi đè Connection hiện có.
- **BR-02:** Credential được lưu qua Secret Store. Domain/database record của Connection chỉ giữ metadata và secret reference.
- **BR-03:** UC-04 chỉ đăng ký và xác minh Connection; provisioning thuộc UC-06 và UC-08.
- **BR-04:** Chỉ Connection READY mới được chọn trong Environment Settings hoặc triển khai.
- **BR-05:** Luồng đăng ký Kubernetes mới nhận kubeconfig qua upload hoặc nội dung được dán; không yêu cầu context có sẵn trên host backend.
- **BR-06:** Giai đoạn đầu hỗ trợ token nhúng hoặc chứng chỉ/private key nhúng, cùng thông tin cluster và cấu hình xác minh TLS cần thiết.
- **BR-07:** Giai đoạn đầu không hỗ trợ credential tham chiếu file bên ngoài hoặc xác thực qua `exec`. Hệ thống không thực thi lệnh từ kubeconfig tải lên.
- **BR-08:** Xác minh Kubernetes kiểm tra API và các quyền trong VAR-01 mà không tạo resource trên cluster.
- **BR-09:** Credential không được xuất hiện trong API đọc, log, thông báo lỗi, snapshot, Preview, manifest hoặc Git. Nội dung kubeconfig không được trả lại qua read model.
- **BR-10:** Xác minh và thực thi chỉ được lấy credential đúng Organization/Connection. Connection mới không fallback sang credential mặc định trên host.
- **BR-11:** Credential AWS dùng cho provisioning không được cấp cho workload. Luồng lưu trữ và sử dụng tuân theo ADR-009.
- **BR-12:** AWS chỉ đạt `READY` sau khi xác minh danh tính, region, Terraform backend và quyền provisioning cần thiết. Đăng nhập hoặc STS identity thành công chưa đủ.
- **BR-13:** Trạng thái `READY` phản ánh kết quả xác minh tại thời điểm đăng ký; không cam kết giám sát kết nối liên tục.
- **BR-14:** Loại Connection và phương thức xác thực phải được phân biệt để có thể bổ sung phương thức xác thực AWS sau này mà không thay đổi ý nghĩa của Connection hoặc tham chiếu từ Resource Definition.
- **BR-15:** Connection host-context hiện có tiếp tục hoạt động theo cơ chế hiện tại; không tự động chuyển đổi hoặc xóa cấu hình cũ.
- **BR-16:** Đăng ký Connection mới không tự động thay đổi connection mặc định của Organization.

## Giao diện giai đoạn đầu

Trang `Platform / Connections` gồm:

- Danh sách Connection `READY` thuộc Organization, hiển thị tên/ID, loại, cluster/endpoint và trạng thái phù hợp với từng loại.
- Form đăng ký Kubernetes với tên kết nối và lựa chọn tải file hoặc dán kubeconfig.
- Danh sách context khi kubeconfig có nhiều context.
- Thông tin context, cluster và endpoint được chọn.
- Nút **Kiểm tra và lưu**.
- Trạng thái đang xử lý, thông báo thành công và lỗi có hướng dẫn xử lý.

Form không yêu cầu nhập Connection ID hoặc Cluster ID. AWS registration chưa
xuất hiện trong giao diện giai đoạn đầu.

## Ngoài phạm vi giai đoạn đầu

- **OOS-01:** Triển khai AWS onboarding, AWS verification và executor credential resolution.
- **OOS-02:** Tạo cluster Kubernetes từ form đăng ký.
- **OOS-03:** Xác thực Kubernetes qua `exec` hoặc file credential bên ngoài.
- **OOS-04:** Sửa/xóa Connection và chuyển đổi Connection host-context hiện có.
- **OOS-05:** Credential rotation, health monitoring, reconnect và audit history.
- **OOS-06:** Humanitec-style Agent, private-cluster tunnel, GitOps mode và cài đặt Operator tự động.

## Ranh giới tài liệu

Trạng thái triển khai thực tế được theo dõi tại [CURRENT_STATE.md](../../CURRENT_STATE.md).
Realization, diagrams, contracts, shared design và traceability sẽ được cập nhật
theo specification sau khi bản này được duyệt.

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
  thường, key ổn định `platform-vault`, name `Platform Vault`, legacy=false.
  Bootstrap chạy cùng validation/verifier và platform credential persistence với
  registration; READY chỉ sau probe/cleanup thành công. Token file chỉ là input
  của bootstrap, không phải cơ chế runtime riêng của store. Restart idempotent,
  không tạo store/credential dư thừa, không tự chọn cho Environment mới.
- **SS-08:** Upgrade kho Compose cũ giữ nguyên ID/key, endpoints/mounts và mọi
  Environment/revision/value/bundle refs; chỉ chuyển legacy record sau verify và
  persist credential thành công. Mismatch identity hoặc failure phải fail closed,
  không overwrite kho người dùng. Legacy configuration ngoài Compose vẫn tương thích.

Supporting flow/design: [ADR-012](../../architecture/decisions/ADR-012-environment-stores-and-transitions.md).
