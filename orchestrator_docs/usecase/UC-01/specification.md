---
id: UC-01-SPEC
artifact: use-case-specification
status: current
last_reviewed: 2026-10-07
---

# UC-01 — Create Application and configure Environment target

## Mục tiêu và actor

Developer tạo Application rồi chọn Connection riêng trong Settings từng Environment.
Organization và role lấy từ session UC-00. Platform Engineer đăng ký Connection
và matching Definitions qua UC-03/04.

## Tiền điều kiện

- **PRE-01:** Organization đã tồn tại.
- **PRE-02:** Developer đã xác thực và thuộc Organization.
- **PRE-03:** Platform đã cấu hình base domain; tạo Application không cần default Connection.
- **PRE-04:** Khi set Environment target, có Connection được hỗ trợ, `READY` trong Organization.

## Trigger và main success scenario

- **TRG-01:** Developer yêu cầu tạo Application.
1. **MS-01:** Developer nhập Name và Subdomain; không chọn Connection ở cấp Application.
2. **MS-02:** Orchestrator validate Name/Subdomain và uniqueness.
3. **MS-03:** Orchestrator sinh Application ID bất biến.
4. **MS-04:** Tạo Application với configuration provider của platform, không execution target.
5. **MS-05:** Tạo hai Environment `staging` và `production`, target `UNCONFIGURED`.
6. **MS-06:** Khởi tạo empty Deployment Sets và stable namespace identity cho mỗi Environment.
7. **MS-07:** Suy ra desired endpoints, chưa provision infrastructure/workloads/routes.
8. **MS-08:** Persist atomically và mở Application home với staging được chọn.

## Environment Settings flow

- **TRG-02:** Developer mở Settings của Environment đang chọn.
- **ES-01:** Hiển thị target chưa cấu hình hoặc binding đã lưu chỉ đọc.
- **ES-02:** Với `UNCONFIGURED`, load safe READY choices của Organization, hiển thị tên/key/kind và default marker. Không tự set default; chọn trên UI chưa persist.
- **ES-03:** Developer chọn một Connection và bấm `Set connection`; thông báo rõ lựa chọn không thể đổi sau khi lưu.
- **ES-04:** Backend kiểm tra Organization, READY, kind/region và expected Environment version trong transaction.
- **ES-05:** Atomic set-once lưu connection/profile/region/runtime status/infrastructure scope, tăng Environment version; trả binding chỉ đọc.
- **ES-06:** Environment kia giữ nguyên; refresh/restart vẫn khóa binding đã set.

## Hậu điều kiện

- **POST-01:** Application thuộc Organization; system ID bất biến.
- **POST-02:** Đúng hai Environment staging/production.
- **POST-03:** Mỗi Environment có empty Set, stable namespace và desired endpoint.
- **POST-04:** UC-01 không gọi executor hoặc provision runtime.
- **POST-05:** Chỉ Environment đã set target mới Preview/Deploy; draft và variables/secrets có thể chuẩn bị trước.

## Quy tắc nghiệp vụ

- **BR-01:** Application ID do hệ thống sinh, bất biến, unique toàn cục.
- **BR-02:** Name unique không phân biệt hoa thường trong Organization; create nhận đúng Name/Subdomain.
- **BR-03:** Subdomain là DNS label lowercase, unique trong platform base domain.
- **BR-04:** Application có đúng hai Environment hệ thống tạo staging/production.
- **BR-05:** Endpoints `<subdomain>.<base-domain>` và `staging.<subdomain>.<base-domain>`.
- **BR-06:** Desired endpoint chưa provision cho đến UC-06.
- **BR-07:** Set Connection chỉ chấp nhận READY Kubernetes/AWS của session Organization; AWS cần region không rỗng. Backend suy profile/region; không nhận profile/region/credential/Organization từ request. Missing/foreign/not-ready/unsupported đều safe `422` field `connectionKey`; blank/null/missing key `400`. Không fallback default.
- **BR-08:** Binding thuộc Environment và chỉ được set thành công một lần, kể cả chưa deploy. Mọi request set lại, kể cả cùng key, trả `409`; không có reset/unset/replace. Hai Environment độc lập, được khác connection/kind/profile/region. Đổi Organization default hoặc đăng ký Connection không thay binding.
- **BR-09:** AWS VPC/EKS mới có Environment scope và identity riêng; không reuse hạ tầng staging cho production. Migration giữ nguyên target và identity application-scope của legacy AWS bằng marker `LEGACY_APPLICATION`, không migrate/destroy/reprovision tài nguyên cũ. Xem [ADR-011](../../architecture/decisions/ADR-011-environment-execution-binding.md).
- **BR-10:** Namespace identity Environment ổn định; connection selection không đổi namespace.
- **BR-11:** Atomic compare-and-set theo expected Environment version, unset binding và Organization ownership. Concurrent sets chỉ một thành công; stale request `409`; rejection không mutate. Normal save/current-set/runtime operations không được ghi đè binding.
- **BR-12:** Application/JSON/SQL cũ được backfill binding xuống từng Environment một lần, giữ nguyên profile/region/runtime và namespace, coi là đã set/khóa kể cả chưa từng deploy. Migration idempotent; không backfill default vào Application mới chưa cấu hình.
- **BR-13:** API create mới reject `connectionKey` như unknown field `400`; không giữ luồng tự set hai Environment từ create. Old persisted data được giữ; old create clients phải cập nhật. Safe Application read/list trả target trong từng Environment, không đại diện bằng một Application target.
- **BR-14:** UNCONFIGURED Preview/Deploy trả safe `422` field `connectionKey` và UI dẫn tới Environment Settings; không gọi executor, không tạo provisioning/deployment side effects. Changes to binding version/hash invalidate prior previews; snapshot/plan pins selected target.

## Ngoài phạm vi

Sửa Name/Subdomain; thêm/xóa/clone/promotion Environment; chuyển target sau set;
AWS credential onboarding mới (giữ scope ADR-009 hiện tại); AWS cloud execution
không nằm trong kiểm chứng này. UC-12 sở hữu variables/secrets, không sở hữu target binding.

## Delivery state

Thiết kế per-Environment set-once được chốt 2026-10-07; implementation cũ cấp
Application đang được thay thế. Evidence ngày trước mô tả behavior lịch sử.
