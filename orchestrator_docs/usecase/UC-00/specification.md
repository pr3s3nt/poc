---
id: UC-00-SPEC
artifact: use-case-specification
status: current
last_reviewed: 2026-10-29
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

- **BR-01:** MVP chỉ sử dụng internal account trong database; không có
  self-registration. Chỉ Admin tạo account mới (BR-09) và Admin đầu tiên được
  khởi tạo bằng lệnh CLI (BR-10).
- **BR-02:** Password chỉ được persist dưới dạng hash.
- **BR-03:** Account có một trong các role cơ bản: `ADMIN`,
  `PLATFORM_ENGINEER`, `DEVELOPER`.
- **BR-04:** Account `DISABLED` không được đăng nhập.
- **BR-05:** Fixed test account chỉ tồn tại trong profile `local`/`test`, không
  được seed hoặc chấp nhận trong production.
- **BR-06:** Session là opaque, được gắn với đúng User, Organization và role;
  token thô không được persist hoặc ghi log.
- **BR-07:** User có thể đăng xuất để kết thúc session.
- **BR-08:** Mọi User đã đăng nhập đổi được password của chính mình: nhập
  password hiện tại, password mới và xác nhận password mới. Đổi thành công thu
  hồi mọi session hiện có của account, yêu cầu đăng nhập lại.
- **BR-09:** Chỉ account role `ADMIN` dùng được quản trị người dùng: xem danh
  sách account của Organization và tạo account với username, password và role.
  Username là duy nhất; dữ liệu không hợp lệ bị từ chối; backend tự kiểm tra
  quyền `ADMIN` ở API, không tin role do client gửi.
- **BR-10:** Admin đầu tiên được khởi tạo bằng lệnh CLI của orchestrator với
  username và file chứa password; không có password Admin mặc định trong code.
- **BR-11:** Chỉ account role `ADMIN` đặt lại được password của một account
  trong cùng Organization: nhập password mới và xác nhận password mới. Password
  mới phải thỏa chính sách password tối thiểu; đặt lại thành công thu hồi mọi
  session của account đó, yêu cầu đăng nhập lại. Backend tự kiểm tra quyền
  `ADMIN` ở API, không tin role do client gửi; Admin không đọc được password
  hiện tại của account khác.

## Ngoài phạm vi MVP

- **OOS-01:** Vô hiệu hóa hoặc đổi role account.
- **OOS-02:** MFA, self-service password reset (quên mật khẩu qua email) và
  email verification; Admin đặt lại password theo BR-11 nằm trong phạm vi.
- **OOS-03:** OAuth, OIDC, SAML hoặc external IdP.
- **OOS-04:** Fine-grained RBAC, audit login và account lockout.
