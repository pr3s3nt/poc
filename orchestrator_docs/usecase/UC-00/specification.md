---
id: UC-00-SPEC
artifact: use-case-specification
status: current
last_reviewed: 2026-09-23
---

# UC-00 — Sign In

## Mục tiêu

Cho phép User đăng nhập bằng internal account để sử dụng Orchestrator trong
Organization của họ.

## Primary actor

- User

## Tiền điều kiện

- **PRE-01:** User có internal account cố định trong database, trạng thái
  `ACTIVE`.
- **PRE-02:** Môi trường `local`/`test` đã seed các test account cần thiết.

## Trigger

**TRG-01:** User mở màn hình đăng nhập.

## Main success scenario

1. **MS-01:** User nhập username và password.
2. **MS-02:** Orchestrator kiểm tra thông tin đăng nhập của internal account.
3. **MS-03:** Orchestrator xác định User, Organization và role của account.
4. **MS-04:** Orchestrator tạo session đăng nhập.
5. **MS-05:** Orchestrator chuyển User vào ứng dụng với quyền phù hợp.

## Hậu điều kiện

- **POST-01:** User có session hợp lệ.
- **POST-02:** Các use case tiếp theo dùng identity, Organization và role từ
  session; UI không tự truyền Organization ID hoặc role.

## Quy tắc nghiệp vụ

- **BR-01:** MVP chỉ sử dụng fixed internal account được seed sẵn; không có
  self-registration hoặc account-management flow.
- **BR-02:** Password chỉ được persist dưới dạng hash.
- **BR-03:** Account có một trong các role cơ bản: `ADMIN`,
  `PLATFORM_ENGINEER`, `DEVELOPER`.
- **BR-04:** Account `DISABLED` không được đăng nhập.
- **BR-05:** Fixed test account chỉ tồn tại trong profile `local`/`test`, không
  được seed hoặc chấp nhận trong production.
- **BR-06:** Session là opaque, được gắn với đúng User, Organization và role;
  token thô không được persist hoặc ghi log.
- **BR-07:** User có thể đăng xuất để kết thúc session.

## Ngoài phạm vi MVP

- **OOS-01:** Tạo, vô hiệu hóa hoặc đổi role account.
- **OOS-02:** MFA, password reset và email verification.
- **OOS-03:** OAuth, OIDC, SAML hoặc external IdP.
- **OOS-04:** Fine-grained RBAC, audit login và account lockout.
