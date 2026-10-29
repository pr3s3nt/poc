# Acceptance application deploy

## Mục tiêu

Developer triển khai acceptance application hoàn toàn qua màn hình Web Console:
frontend, backend, worker dùng chung một PostgreSQL và một Redis. Script tạo
Application, chọn Secret Store, lưu biến và bí mật, nhập ba Score, xem trước,
triển khai và đọc kết quả trên màn hình. Liên quan:
[UC-01](../../../orchestrator_docs/usecase/UC-01/README.md),
[UC-05](../../../orchestrator_docs/usecase/UC-05/README.md),
[UC-06](../../../orchestrator_docs/usecase/UC-06/README.md),
[UC-08](../../../orchestrator_docs/usecase/UC-08/README.md),
[UC-12](../../../orchestrator_docs/usecase/UC-12/README.md),
[staging catalog](../../../orchestrator_docs/operations/staging-catalog.md).

## Prerequisites

- Console thật (không fake adapter), cấu hình chung theo
  [e2e/README.md](../../README.md#dependencies-và-kết-nối-console-dùng-chung).
  Biến `ORCH_PLATFORM_*` dùng cho đăng nhập; scenario đăng nhập bằng
  developer.
- Đã chạy [platform setup](../../setup/README.md): Internal account `k8s-4f`,
  catalog `staging-k8s-4f-*`, Secret Store Sẵn sàng có Kubernetes auth cho
  đúng cluster.
- Image `acceptance-frontend|backend|worker:<tag>` cluster kéo được. Với kind
  local, build và nạp thẳng vào node:

  ```bash
  cd backend
  bash test/integration/build-images.sh <tag>
  kind load docker-image acceptance-frontend:<tag> acceptance-backend:<tag> \
    acceptance-worker:<tag> --name k8s-4f
  ```

  Cluster khác cần push lên registry và đặt `ORCH_ACCEPTANCE_IMAGE_PREFIX`.

## Run

Từ thư mục `e2e`:

```bash
ORCH_PLATFORM_URL=http://localhost:3001 \
ORCH_PLATFORM_USERNAME=platform-engineer \
ORCH_PLATFORM_PASSWORD_FILE=~/.orch/local-password \
ORCH_ACCEPTANCE_IMAGE_TAG=<tag> \
ORCH_KUBE_CONTEXT=kind-k8s-4f \
npm run acceptance-app
```

| Biến | Mặc định | Ý nghĩa |
|---|---|---|
| `ORCH_ACCEPTANCE_IMAGE_TAG` | bắt buộc | Tag image acceptance |
| `ORCH_ACCEPTANCE_IMAGE_PREFIX` | rỗng | Registry prefix, ví dụ `registry.example.com/team/` |
| `ORCH_APP_NAME` | `acceptance-<yymmddhhmm>` | Tên và subdomain Application mới |
| `ORCH_DEVELOPER_USERNAME` | `developer` | Tài khoản developer |
| `ORCH_DEVELOPER_PASSWORD_FILE` | file của `ORCH_PLATFORM_PASSWORD_FILE` | Mật khẩu developer |
| `ORCH_SECRET_STORE_NAME` | `K8S-4F Vault` | Kho bí mật chọn cho staging |
| `ORCH_KUBE_CONTEXT` | rỗng | Có giá trị thì mở trang ứng dụng qua port-forward để kiểm tra |
| `ORCH_APP_NAMESPACE` | `app-<application-id>-staging` | Namespace của frontend khi port-forward |
| `ORCH_WORKLOAD_DELAY_MS` | `2000` | Dừng trước/sau mỗi thao tác tạo workload; 0..10000 ms. Sau import, xem form nếu giữ được đầy đủ Score; nếu form khóa thì đọc cấu hình import |
| `ORCH_EVIDENCE_DIR` | rỗng | Thư mục rỗng ngoài repository để lưu video Console/app, ảnh cuối và result.json; giữ cả khi FAIL |
| `ORCH_HEADLESS=1` | không | Chạy không hiện cửa sổ trình duyệt |

## Steps and assertions

1. Đăng nhập developer, vai trò `DEVELOPER`.
2. Tạo Application; tên đã có trên danh sách thì dừng.
3. Cài đặt môi trường staging: chọn Secret Store, lưu 8 biến và bí mật
   `ACCEPTANCE_SECRET` sinh ngẫu nhiên; biến `ACCEPTANCE_SECRET_SHA256` là
   SHA-256 của bí mật. Bí mật hiện trên trang thì FAIL.
4. Nhập Score backend, worker, frontend từ [scores/](scores/) bằng ô chọn file.
5. Xem trước thay đổi: màn hình báo 3 Workload bị ảnh hưởng.
6. Triển khai: màn hình báo Triển khai thành công và từng workload thành công.
7. Có `ORCH_KUBE_CONTEXT`: mở trang acceptance app, chờ 5 dòng kiểm tra PASS
   (backend connection, environment, secret, database, cache), gửi job bằng
   form của ứng dụng và chờ DONE.

PASS in đường dẫn Application và exit 0; lỗi in `FAIL: <lý do>`, exit 1.

## Side effects and cleanup

Tạo Application, cấu hình staging và trên cluster: namespace, PostgreSQL,
Redis, ba workload. Kết quả được **giữ lại** để người dùng xem; script không
xóa. Chạy lại tạo Application mới với tên khác. Fail giữa chừng có thể để lại
Application dở dang: xem trên Console, gỡ workload và tài nguyên qua Console.

## Evidence

Mặc định chỉ output terminal. Đặt `ORCH_EVIDENCE_DIR` để ghi toàn bộ Console
và trang acceptance app thành `console.webm`, `application.webm` (1440×1000),
ảnh cuối và `result.json` chứa Application ID, URL, tag và assertions. Browser
tự đóng sau PASS/FAIL để chốt clip. Video trong `raw/`
là bản gốc; tất cả evidence nằm ngoài Git. Mật khẩu và secret chỉ nhập trong ô
password; không bật show password, trace hay log response chứa secret.

Ví dụ chạy chậm và quay video từ root repo:

```bash
ORCH_PLATFORM_URL=http://localhost:3001 \
ORCH_PLATFORM_USERNAME=platform-engineer \
ORCH_PLATFORM_PASSWORD_FILE=~/.orch/local-password \
ORCH_ACCEPTANCE_IMAGE_TAG=<tag> ORCH_KUBE_CONTEXT=kind-k8s-4f \
ORCH_HEADLESS=1 ORCH_WORKLOAD_DELAY_MS=2500 \
ORCH_EVIDENCE_DIR="$HOME/.local/state/orch-acceptance/<run-id>" \
node e2e/scenarios/acceptance-app/run.mjs
```

Port-forward chỉ tồn tại trong lượt chạy. Không có context thì chỉ chứng minh
Console deploy; không coi đó là PASS các kiểm tra trên ứng dụng. Import đi qua
UI và validation chuẩn; không dùng API để ghi draft hoặc deploy. Kịch bản không
tự sửa code/retry khi lỗi: agent/operator chẩn đoán, sửa, kiểm tra rồi chạy tên
Application mới; giữ lại Application lỗi để kiểm tra, không đụng resource cũ.

Khi người dùng đã yêu cầu upload, chuyển clip sang MP4 và kiểm tra bằng ffmpeg,
xem các frame để xác nhận không lộ bí mật, rồi upload asset có tên run riêng lên
GitHub release `acceptance-recordings`. Không commit video vào Git, không upload
credential/session/log. Không dùng `--clobber` để ghi đè clip của lượt khác.
