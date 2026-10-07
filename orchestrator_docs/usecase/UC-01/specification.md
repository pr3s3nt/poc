---
id: UC-01-SPEC
artifact: use-case-specification
status: current
last_reviewed: 2026-10-07
---

# UC-01 — Create Application

## Mục tiêu

Cho phép Developer tạo Application để khai báo và deploy workload.

## Primary actor

- Developer

## Supporting actor

- Platform Engineer
- Organization Administrator

## Tiền điều kiện

- **PRE-01:** Organization đã tồn tại.
- **PRE-02:** Developer đã được xác thực và thuộc Organization.
- **PRE-03:** Platform đã cấu hình base domain và default execution target cho Organization.
- **PRE-04:** Organization có ít nhất một Connection được hỗ trợ, trạng thái `READY`.

## Trigger

**TRG-01:** Developer yêu cầu tạo một Application.

## Main success scenario

1. **MS-01:** Developer nhập Application Name, Subdomain và chọn Connection. Default của Organization được chọn sẵn nếu nằm trong danh sách hợp lệ.
2. **MS-02:** Orchestrator kiểm tra thông tin hợp lệ và Subdomain chưa được sử dụng trong base domain platform.
3. **MS-03:** Orchestrator tự tạo Application ID duy nhất và resolve Connection được chọn trong Organization của session; nếu request cũ bỏ `connectionKey`, resolve default của Organization.
4. **MS-04:** Orchestrator tạo Application với runtime configuration của target đó.
5. **MS-05:** Orchestrator tự tạo hai Environment `staging` và `production` thuộc Application.
6. **MS-06:** Orchestrator khởi tạo Deployment Set rỗng và namespace identity riêng cho mỗi Environment.
7. **MS-07:** Orchestrator hiển thị desired endpoint cho mỗi Environment.
8. **MS-08:** Orchestrator lưu Application và Environments, rồi trả về Application context cho Developer.

## Luồng biến thể trong happy path

- **VAR-01 — `aws-eks`:** Connection được chọn cung cấp AWS connection và region; UC-01 chưa tạo VPC/EKS.
- **VAR-02 — `internal-k8s`:** Connection được chọn cung cấp Kubernetes connection đang `READY`.

## Hậu điều kiện

- **POST-01:** Application thuộc Organization của Developer và có Application ID do hệ thống sinh.
- **POST-02:** Application có đúng hai Environment: `staging` và `production`.
- **POST-03:** Mỗi Environment có Deployment Set rỗng, namespace identity riêng và desired endpoint.
- **POST-04:** Chưa có workload hoặc runtime infrastructure nào được provision.
- **POST-05:** Application và Environment sẵn sàng được sử dụng trong UC-05 và UC-06.

## Quy tắc nghiệp vụ

- **BR-01:** Application ID do hệ thống sinh, bất biến và duy nhất toàn cục.
- **BR-02:** Developer nhập Application Name, Subdomain và chọn Connection; Name là duy nhất trong Organization.
- **BR-03:** Subdomain là DNS label hợp lệ, được chuẩn hóa lowercase và duy nhất trong base domain platform.
- **BR-04:** Mỗi Application có đúng hai Environment hệ thống tạo: `staging` và `production`.
- **BR-05:** Production desired endpoint là `<subdomain>.<base-domain>`; staging desired endpoint là `staging.<subdomain>.<base-domain>`.
- **BR-06:** Desired endpoint chưa được provision trong UC-01; UC-06 tạo hoặc cập nhật route/ingress sau khi workload sẵn sàng.
- **BR-07:** Developer được chọn Connection `READY` thuộc Organization của session (kind Kubernetes hoặc AWS). Backend suy ra Execution Profile và region từ Connection; không nhận Organization, role, profile, region hoặc credential từ request. Connection không tồn tại/ngoài Organization trả cùng lỗi an toàn `422` với field `connectionKey`; not-ready hoặc kind không hỗ trợ cũng trả `422`. Không fallback khi một key tường minh không hợp lệ. AWS cần region không rỗng. Danh sách lựa chọn loại AWS thiếu region; `defaultConnectionKey` trong response là key nếu default đủ điều kiện, nếu không là chuỗi rỗng. API cũ bỏ key vẫn dùng default; key rỗng hoặc null tường minh trả `400` với field `connectionKey`.
- **BR-08:** Connection và Execution Profile được lưu ở Application, dùng chung cho mọi Environment. UC-01 không cung cấp thao tác đổi connection/profile của Application đã tồn tại; đăng ký Connection hoặc thay default không đổi binding đã lưu.
- **BR-09:** `aws-eks` dùng VPC/EKS scope Application; `internal-k8s` dùng cluster đã đăng ký.
- **BR-10:** Namespace identity có scope Environment và ổn định qua các deployment.

## Luồng nội bộ

```text
UC-01 Create Application
├── Validate Application Name and Subdomain
├── Generate Application ID
├── Resolve selected Organization-scoped Connection (legacy omission uses default)
├── Create Application
├── Create staging and production Environments
├── Initialize Deployment Sets and namespace identities
├── Derive desired endpoints
└── Persist configuration
```

## Trạng thái implementation hiện tại

- Authenticated API/Web Console implement selected connection creation and safe
  Organization-scoped choices (2026-10-07); omitted-key API callers still use
  default. Shared Application binding survives backend restart. See
  [verification](../../verification/2026-10-07-application-connection-selection-local.md).
- Một transaction tạo Application, đúng hai Environment `staging`/`production`
  và hai Deployment Set rỗng. Desired endpoint được UI suy ra từ Subdomain và
  platform base domain; UC-01 không provision infrastructure hoặc workload.
- In-memory/JSON adapter phục vụ local/test; normalized PostgreSQL adapter đã
  persist Application/Environment/Deployment Set và giữ state qua backend
  restart. Seed `acceptance` với Environment `dev` vẫn là planning fixture độc
  lập, không đại diện cho output UC-01.
- Planning context dùng Application, Environment, profile, connection, region,
  namespace identity và runtime status; VPC/EKS có application scope.

## Ngoài phạm vi happy path

- **OOS-01:** Sửa Name hoặc Subdomain sau khi Application được tạo.
- **OOS-02:** Thêm, xóa, đổi tên, clone hoặc promotion Environment.
- **OOS-03:** Đổi connection của Application đã tồn tại hoặc quản lý Connection/credential/cluster.
- **OOS-04:** DNS ownership verification, certificate lifecycle, custom domain và external DNS provisioning.
- **OOS-05:** RBAC chi tiết, concurrent update và audit history.
