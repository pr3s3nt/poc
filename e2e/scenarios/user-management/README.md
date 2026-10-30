# user-management — UC-00 BR-08/09/10/11

Kiểm chứng quản trị người dùng ở mức API (shell + curl), không cần browser.

## Mục tiêu

- [UC-00 specification](../../../orchestrator_docs/usecase/UC-00/specification.md)
  — BR-08 (đổi mật khẩu, thu hồi mọi session), BR-09 (quản trị người dùng chỉ
  cho Admin), BR-10 (lệnh khởi tạo Admin đầu tiên), BR-11 (Admin đặt lại mật
  khẩu, thu hồi mọi session của account đó).

## Prerequisites

- Go toolchain để build `backend/cmd/orchestrator`.
- `curl`, `bash`.
- Không cần platform, database, Kubernetes hay browser; backend chạy với fake
  adapters và JSON state trong thư mục tạm (fixture, không phải adapter thật).
- Cổng `18099` (đổi qua `ORCH_E2E_PORT`).

## Cách chạy

```bash
bash e2e/scenarios/user-management/run.sh
```

Không cần biến cấu hình bắt buộc. Mật khẩu kiểm thử được tạo trong thư mục tạm
riêng tư (0600) của lượt chạy, không đọc từ môi trường người dùng.

## Các bước và assertions

1. Build binary orchestrator vào thư mục tạm.
2. `-bootstrap-admin` với file mật khẩu tạm: PASS khi account `admin` được tạo;
   output không chứa mật khẩu; chạy lại bị từ chối (username trùng); chạy không
   có `-state`/`-database-url-file` bị từ chối (store tạm).
3. Khởi động API từ state đó, chờ `/api/v1/healthz`.
4. BR-09: anonymous 401; DEVELOPER bị 403 khi list/tạo; Admin list đủ tài khoản
   không lộ hash; tạo trùng username 409 `DUPLICATE`; username/role/password
   không hợp lệ 400; tạo `newdev` 201 và đăng nhập được.
5. BR-08: sai mật khẩu hiện tại 401; xác nhận không khớp 400; mật khẩu mới
   ngắn 400; các lần thất bại giữ nguyên session; đổi thành công 204 → cả hai
   session của account đều 401, session account khác còn sống; mật khẩu cũ bị
   từ chối, mật khẩu mới đăng nhập được.
6. BR-11: non-Admin 403, anonymous 401; xác nhận không khớp 400; mật khẩu mới
   ngắn 400; account không tồn tại 404; các lần thất bại giữ nguyên session của
   target; Admin đặt lại thành công 204 → cả hai session của target đều 401,
   session Admin còn sống; mật khẩu trước reset bị từ chối, mật khẩu mới đăng
   nhập được.

Mọi bước in `PASS:`/`FAIL:`; script exit 1 khi có FAIL và kèm 20 dòng cuối log
server.

## Side effects

Chỉ một process orchestrator local trên `127.0.0.1:18099` và thư mục tạm của
lượt chạy. Chạy lại được ngay; thất bại giữa chừng không để lại trạng thái
ngoài thư mục tạm.

## Cleanup

`trap EXIT` kill process server và xóa toàn bộ thư mục tạm (state JSON, cookie
jar, mật khẩu tạm, binary). Không giữ dữ liệu nào sau lượt chạy.

## Evidence và giới hạn

Output của lượt chạy là evidence; log server chỉ in khi FAIL. Kịch bản kiểm chứng
hợp đồng API và hành vi session; giao diện Web Console của các luồng này được
kiểm bằng component tests trong `frontend/src/features/users` và
`frontend/src/features/auth`, không phải scenario này.
