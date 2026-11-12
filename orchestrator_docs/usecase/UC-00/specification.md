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
- **OOS-03:** SAML và external IdP ngoài OIDC đã cấu hình ở SSO bên dưới.
- **OOS-04:** Fine-grained RBAC, audit login và account lockout.

## Organization identity for installation seed — issue #47

An external seed configuration may declare `organization: {key, name}`. A local
preparation step exports only that identity, without credentials. Before store
bootstrap, the backend accepts the identity via an operator-owned file and uses
it to create a missing Organization. A matching existing Organization retains
its ID and name; an explicit name mismatch fails before other seed writes. No
rename, reassignment of accounts or public Organization-creation API is added.
Identity key is a lowercase DNS label (1–63 characters); display name is
nonblank, trimmed, at most 200 Unicode characters and contains no control
characters. Legacy installations without an identity file retain the existing defaults.
Authenticated `GET /api/v1/organization` returns only the session Organization.
The API importer verifies both key and name before platform writes. Different
login Organization or name is a conflict, never an instruction to migrate data.

## Keycloak SSO — issue #77

- **SSO-01:** Installation chọn local account hoặc OIDC bằng cấu hình server.
  OIDC dùng một issuer, confidential client, redirect URI cố định và Organization
  đã tồn tại. Khi bật OIDC, local sign-in và các API quản lý/password nội bộ bị
  từ chối; quản lý nhân viên/password/role tại Keycloak.
- **SSO-02:** Console giữ giao diện đăng nhập hiện hành, thay form local bằng nút
  đăng nhập SSO. Backend dùng Authorization Code + PKCE S256, state và nonce
  ngẫu nhiên; callback chỉ chấp nhận giao dịch chưa dùng, còn hạn 5 phút và
  cookie giao dịch đúng browser. Xác minh chữ ký, issuer, audience, expiry,
  nonce và subject. Redirect sau đăng nhập về `/ui/`, không nhận return URL.
- **SSO-03:** Danh tính ngoài được xác định bởi issuer + subject; ID nội bộ được
  suy ra ổn định từ cặp này. Lần đầu tạo account ACTIVE không có mật khẩu local,
  gắn Organization server đã cấu hình. Không ghép account bằng email/username;
  username trùng account khác bị từ chối. Chưa có migration tài khoản cũ.
- **SSO-04:** Chỉ đọc client roles của client đã cấu hình. Mapping mặc định:
  developer → DEVELOPER, platform-engineer → PLATFORM_ENGINEER, admin → ADMIN.
  Mapping thay đổi qua cấu hình. Nhiều role hợp lệ chọn ADMIN rồi
  PLATFORM_ENGINEER rồi DEVELOPER; không có role hợp lệ thì từ chối.
  Tái sử dụng mọi authorization/Organization guard của các use case hiện có.
- **SSO-05:** Platform tiếp tục dùng opaque HttpOnly SameSite=Lax cookie,
  Secure với public URL HTTPS, TTL tối đa 8 giờ. Token OIDC chỉ ở bộ nhớ backend;
  restart backend yêu cầu đăng nhập lại, account/domain data được giữ.
  Mỗi phiên refresh và xác minh role tối đa mỗi 30 giây khi có request; token
  hết hạn phải refresh sớm hơn. Keycloak lỗi, refresh bị từ chối, thiếu role hoặc
  subject thay đổi thì kết thúc phiên và từ chối request, không fallback local.
  Đổi role/khóa account được áp dụng ở lần refresh tiếp theo; không tuyên bố
  thu hồi tức thời. Logout cục bộ thu hồi session và refresh token, không tự
  đăng xuất các ứng dụng khác dùng chung SSO.
- **SSO-06:** Keycloak test riêng, ba tài khoản đại diện, role mapping và negative
  cases kiểm tra bằng instance thật. Credential sinh ngoài Git. HTTP chỉ được
  bật rõ cho môi trường test; khi kết nối công ty dùng HTTPS và trust CA chuẩn.
  Không thay đổi Keycloak công ty trong task này.
