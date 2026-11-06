---
id: OPS-TAILSCALE-HANDOFF
artifact: operations-runbook
status: current
last_reviewed: 2026-11-06
---

# Bàn giao ứng dụng để người dùng thử qua Tailscale

Quyết định người dùng ngày 2026-11-02, được ghi bắt buộc trong
[AGENTS.md](../../AGENTS.md): mọi link mời mở thử ứng dụng, Console, UI/mock hoặc
preview phải truy cập được qua Tailscale, trừ yêu cầu rõ khác của người dùng.
Runbook này sở hữu cách xác định và kiểm tra URL, không cấp thêm phạm vi deploy
hay thay đổi dữ liệu ngoài task.

## Xác định URL thực tế

1. Xác định đúng dịch vụ và trang cần thử, port đang chạy và cơ chế truy cập
   hiện có. Phân biệt địa chỉ upstream nội bộ với địa chỉ người dùng sẽ mở.
2. Đọc trạng thái Tailscale và cấu hình Serve hiện tại:

```bash
tailscale status
tailscale ip -4
tailscale serve status
ss -ltn
```

3. Nếu dùng Tailscale Serve, lấy đúng URL MagicDNS, scheme và port mà lệnh status
   công bố. Nếu dùng listener trực tiếp trên interface Tailscale, chỉ dùng IP/port
   sau khi đã kiểm tra URL đó. Không hardcode IP/hostname của máy từ task cũ.
4. Ghép đường dẫn của task vào URL gốc, ví dụ `/ui/platform/catalog` hoặc
   `/ui/applications/<application-id>`. Ghi rõ link mở Console hay web của workload.
   Với web workload, kiểm tra tuyến truy cập tới chính web đó.

Serve có thể định tuyến theo HTTP Host. Một URL dạng IP trả 404 không có nghĩa là
upstream hỏng; kiểm tra URL MagicDNS đã công bố. Tương tự, MagicDNS và port chưa
được kiểm tra không tự trở thành link dùng được chỉ vì máy có địa chỉ Tailscale.

## Kiểm tra đúng đường truy cập sẽ bàn giao

- Gọi đúng URL gốc/route qua Tailscale để kiểm tra kết nối và phản hồi HTTP;
  dùng health endpoint nếu dịch vụ có endpoint đó. Giữ hostname trong URL như
  người dùng sẽ mở, không dùng `Host` override hay request localhost để thay thế.
- Mở chính URL Tailscale bằng browser, hoàn tất đăng nhập nếu cần và xác nhận
  nội dung/trạng thái chính của màn hình đích. Với SPA, HTTP 200 có thể chỉ là
  HTML shell; kiểm tra API cùng origin và dữ liệu cần thiết của trang đã tải.
- Với ứng dụng thật, dùng backend/dữ liệu thật cho kiểm tra truy cập. Nếu task
  được giao là mock/prototype, nêu rõ phạm vi đó; mock không chứng minh backend
  hay workload đã hoạt động.
- Ghi URL, thời điểm, trang/trạng thái đã thấy và phạm vi kiểm tra. Kiểm tra từ
  máy host qua URL Tailscale là bằng chứng ở điểm đó; chỉ nói đã kiểm tra từ máy
  khác trong tailnet khi thực sự đã thực hiện. Không suy khả năng truy cập của
  mọi thiết bị/ACL từ kết quả localhost.

Dùng công cụ hiện có của repo để đăng nhập; không đưa mật khẩu, token hoặc cookie
vào URL hay evidence. Quy ước browser/E2E nằm trong [E2E guide](../../e2e/README.md).

## Giữ ứng dụng sẵn sàng để thử

Ưu tiên endpoint Tailscale hiện có của đúng dịch vụ. Nếu cần thiết lập đường
truy cập trong phạm vi task, giữ cấu hình các dịch vụ khác và dùng cơ chế tailnet
hiện hành. Dịch vụ, proxy hoặc port-forward cần cho link phải còn hoạt động sau
handoff. Ghi cấu hình/lệnh khởi động lại và vị trí dữ liệu/evidence được giữ.
Không dọn forward vừa tạo nếu đó là đường truy cập duy nhất của link đã đưa.

Nếu chưa xác định được endpoint, Serve trỏ sai upstream, browser/API lỗi hoặc
thiếu khả năng cấu hình kết nối, xử lý phần được phép và báo rõ trở ngại thực tế.
Không đánh dấu “sẵn sàng để thử” bằng cách gửi một URL loopback hoặc URL chưa kiểm tra.

## Bản thử của tính năng

Quyết định người dùng ngày 2026-11-06: mỗi tính năng chạy một stack Docker riêng
và có link `http://<tinh-nang>.<domain>:8088/ui/`. MagicDNS không có tên con nên
`<domain>` mặc định là tên sslip.io chứa IP Tailscale của WSL. Dùng domain riêng
thì trỏ wildcard DNS về IP đó và đổi `FEATURE_DOMAIN`. Link chỉ mở được trong
tailnet và dùng HTTP. Docker trên máy phát triển chạy rootless, không bind được IP
Tailscale, nên proxy nghe ở loopback và `socat` chuyển tiếp từ IP Tailscale.

Chạy trong WSL từ checkout của nhánh tính năng. `FEATURE` là tên nhánh bỏ
`features/`, viết thường, thay `_` bằng `-`, vì đây là một nhãn DNS.

```bash
FEATURE=ten-tinh-nang
TS_IP=$(ip -4 -o addr show tailscale0 | awk '{print $4}' | cut -d/ -f1)
FEATURE_DOMAIN=${TS_IP//./-}.sslip.io

# Proxy và forward dùng chung, chạy một lần cho cả máy
docker compose -f deploy/local/compose.feature-proxy.yml up -d
ss -ltn | grep -q "$TS_IP:8088 " || setsid nohup socat \
  "TCP4-LISTEN:8088,bind=$TS_IP,reuseaddr,fork" TCP4:127.0.0.1:8088 \
  >/dev/null 2>&1 </dev/null &

FEATURE=$FEATURE FEATURE_DOMAIN=$FEATURE_DOMAIN \
BACKEND_IMAGE=poc-feature-backend:$FEATURE \
FRONTEND_IMAGE=poc-feature-frontend:$FEATURE \
  docker compose -p "poc-$FEATURE" -f docker-compose.yml \
  -f deploy/local/compose.feature-url.yml up -d --build

# Stack mới chưa có tài khoản: tạo admin, mật khẩu chỉ lưu ngoài Git
PW=~/.config/poc-feature/$FEATURE-admin-password
mkdir -p -m 700 "$(dirname "$PW")"
(umask 077; openssl rand -base64 18 | tr -d '/+=' > "$PW")
docker exec -i "poc-$FEATURE-backend-1" sh -c 'umask 077; cat > /tmp/admin-password' < "$PW"
docker exec "poc-$FEATURE-backend-1" sh -c '/usr/local/bin/orchestrator -bootstrap-admin \
  -bootstrap-admin-password-file /tmp/admin-password; rc=$?; rm -f /tmp/admin-password; exit $rc'
```

Kiểm tra link theo mục trên, gồm đăng nhập bằng `admin`. `-p` và image riêng giữ
các tính năng tách biệt, và stack không mở port trên máy nên chạy song song được.
`socat` không tự chạy lại sau khi khởi động lại WSL; khi đó chạy lại phần proxy.

Sau khi đóng issue, gỡ stack, image và mật khẩu; `-v` xóa dữ liệu của riêng stack
đó. Dùng cùng các biến ở trên:

```bash
FEATURE=$FEATURE FEATURE_DOMAIN=$FEATURE_DOMAIN \
BACKEND_IMAGE=poc-feature-backend:$FEATURE \
FRONTEND_IMAGE=poc-feature-frontend:$FEATURE \
  docker compose -p "poc-$FEATURE" -f docker-compose.yml \
  -f deploy/local/compose.feature-url.yml down -v
docker image rm "poc-feature-backend:$FEATURE" "poc-feature-frontend:$FEATURE"
rm -f ~/.config/poc-feature/$FEATURE-admin-password
```

## Nội dung bàn giao

Final answer và lời mời mở thử cần có:

- Link Tailscale đầy đủ tới đúng màn hình/ứng dụng của task.
- Kết quả kiểm tra browser ngắn gọn và yêu cầu đăng nhập nếu có, không kèm secret.
- Thông tin dịch vụ/forward hay dữ liệu được giữ nếu ảnh hưởng tới việc mở thử.

`localhost`, `127.0.0.1`, `0.0.0.0`, ClusterIP và địa chỉ mạng Docker có thể được ghi
trong lệnh vận hành hoặc evidence nội bộ; không dùng chúng làm link mời người dùng thử.
Quy tắc này không yêu cầu sửa URL trong các record lịch sử để giả như lượt chạy cũ
đã được kiểm tra qua Tailscale.
