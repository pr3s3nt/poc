# Console sign-in

## Mục tiêu

Mở Web Console theo địa chỉ cấu hình, kiểm tra phiên đăng nhập, đăng nhập nếu
chưa có và xác nhận đúng tài khoản/vai trò. Đây là bước đầu của mọi scenario
khác; chúng dùng cùng helper [`e2e/lib/console.mjs`](../../lib/console.mjs).
Liên quan: [UC-00](../../../orchestrator_docs/usecase/UC-00/README.md),
[chạy Console local](../../../orchestrator_docs/operations/docker-local.md).

## Prerequisites

- Node 24 và một Console đang chạy, thật hoặc local; không dùng fixture/fake.
- Tài khoản đăng nhập Console. Profile local seed `platform-engineer` và
  `developer`, mật khẩu `test-password`; môi trường khác dùng tài khoản riêng.
- File mật khẩu ngoài repository, chỉ chứa mật khẩu, quyền 0600 hoặc 0400.
  Trên WSL, đặt file trong filesystem Linux (ví dụ `~/.orch/`), không đặt dưới
  `/mnt/c` vì ở đó không giữ được quyền 0600.

Cài một lần từ thư mục `e2e`:

```bash
npm ci
npx playwright install chromium
```

## Run

Từ thư mục `e2e`:

```bash
mkdir -p ~/.orch && (umask 077 && printf '%s\n' 'test-password' > ~/.orch/local-password)

ORCH_PLATFORM_URL=http://localhost:3001 \
ORCH_PLATFORM_USERNAME=platform-engineer \
ORCH_PLATFORM_PASSWORD_FILE=~/.orch/local-password \
npm run console-sign-in
```

| Biến | Bắt buộc | Ý nghĩa |
|---|---|---|
| `ORCH_PLATFORM_URL` | Có | Địa chỉ gốc Console; chấp nhận cả dạng có `/ui` |
| `ORCH_PLATFORM_USERNAME` | Có | Tài khoản đăng nhập |
| `ORCH_PLATFORM_PASSWORD_FILE` | Có | File mật khẩu riêng tư |
| `ORCH_HEADLESS=1` | Không | Chạy không hiện cửa sổ trình duyệt |
| `ORCH_EXPECT_ROLE` | Không | `PLATFORM_ENGINEER` hoặc `DEVELOPER`; sai vai trò thì dừng |

Đổi Console chỉ cần đổi `ORCH_PLATFORM_URL`. Mặc định browser hiện lên và giữ
mở cho tới khi người dùng đóng cửa sổ.

## Steps and assertions

1. Kiểm tra cấu hình: URL là origin http/https, có username, file mật khẩu
   riêng tư và không rỗng.
2. Mở `<url>/ui/`, đọc `GET /api/v1/auth/session`.
3. Đã có phiên của đúng tài khoản thì dùng lại; phiên của tài khoản khác thì FAIL.
4. Chưa có phiên thì điền form đăng nhập; HTTP 401 là sai tài khoản/mật khẩu.
5. Xác nhận lại phiên qua API và vai trò nếu có `ORCH_EXPECT_ROLE`.

PASS in `PASS: đã đăng nhập <user> (<role>), tổ chức <org>` và exit 0; mọi lỗi
in `FAIL: <lý do>` và exit 1.

## Side effects and cleanup

Chỉ tạo một phiên đăng nhập trên server; khi kết thúc script gọi sign-out.
Không tạo/sửa dữ liệu platform, Kubernetes hay cloud. Chạy lại an toàn. Nếu
người dùng đóng hẳn browser trước, phiên server tự hết hạn sau 8 giờ.

## Evidence

Chỉ có output terminal. Script không lưu session, trace, screenshot, video và
không in mật khẩu.
