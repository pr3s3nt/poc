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

Chỉ có output terminal. Không lưu session, trace, screenshot, video; không in
mật khẩu, token hay giá trị bí mật. Port-forward chỉ tồn tại trong lượt chạy.
