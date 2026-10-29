# Platform setup scripts

Chuẩn bị platform trước khi chạy scenario. Mọi thao tác và kiểm tra đi qua
màn hình Web Console bằng tài khoản Platform Engineer, với cấu hình chung ở
[e2e/README.md](../README.md#dependencies-và-kết-nối-console-dùng-chung). Setup
không chứng minh ứng dụng deploy thành công.

Chạy từ thư mục `e2e`, theo thứ tự:

| Bước | Script | Kết quả |
|---|---|---|
| 1 | `setup/register-internal-account/run.mjs` | Infrastructure account Internal từ kubeconfig |
| 2 | [`bootstrap-staging-catalog.mjs`](../../frontend/test/e2e/bootstrap-staging-catalog.mjs) | Năm Definitions `staging-k8s-4f-*`; [runbook](../../orchestrator_docs/operations/staging-catalog.md) |
| 3 | `setup/register-secret-store/run.mjs` | Secret Store Vault cho workload |
| 4 | [`setup/register-render-bundles/run.mjs`](register-render-bundles/README.md) | Luồng đầy đủ account/kubeconfig → Resource Types → Resource Definitions → Git render bundles; nhập UI từng trường, chọn tốc độ riêng từng phần |

Script render bundles ghi qua UI và dùng API GET để xác minh pin/cấu hình;
chạy lại chỉ reuse khi toàn bộ cấu hình khớp, không tự sửa Definition.
Ví dụ của script 4 có thể dùng độc lập cho `k8s-4f`: nó kiểm tra account trước,
chỉ upload kubeconfig khi chưa có, rồi chuẩn bị cluster/namespace/storage/
PostgreSQL/Redis Definitions. Không cần chạy bootstrap staging cũ để tạo trùng
Definitions. Xem README của script để đổi target, schema và tốc độ.
Các script account/Secret Store dùng lại mục cùng tên đã Sẵn sàng; mục có cùng tên nhưng khác trạng
thái thì dừng. Không script nào sửa hoặc xóa mục đã có. Fail giữa chừng có thể
để lại mục đã tạo; xem trên Console rồi chạy lại.

## Register Internal account

| Biến | Mặc định | Ý nghĩa |
|---|---|---|
| `ORCH_KUBECONFIG_FILE` | bắt buộc | Kubeconfig riêng tư (0600), một context, endpoint backend truy cập được |
| `ORCH_ACCOUNT_NAME` | `k8s-4f` | Tên và key của account; catalog staging yêu cầu `k8s-4f` |

Kubeconfig chỉ được tải lên qua ô chọn file; nội dung không in ra, không hiện
trên trang. Với kind trên máy local, xuất context và đổi endpoint sang tên
container control plane mà backend Docker thấy được:

```bash
mkdir -p ~/.orch && chmod 700 ~/.orch
(umask 077 && kubectl --context kind-k8s-4f config view --minify --flatten --raw > ~/.orch/k8s-4f.kubeconfig)
kubectl --kubeconfig ~/.orch/k8s-4f.kubeconfig config set-cluster kind-k8s-4f \
  --server=https://k8s-4f-control-plane:6443
```

## Register Secret Store

| Biến | Mặc định | Ý nghĩa |
|---|---|---|
| `ORCH_VAULT_BACKEND_ADDRESS` | bắt buộc | Địa chỉ Vault mà backend Console truy cập |
| `ORCH_VAULT_WORKLOAD_ADDRESS` | như backend | Địa chỉ Vault mà Pod/VSO trong cluster truy cập |
| `ORCH_VAULT_TOKEN_FILE` | bắt buộc | File riêng tư chứa token Vault có phạm vi, không dùng root token |
| `ORCH_SECRET_STORE_NAME` | `K8S-4F Vault` | Tên kho trên Console |
| `ORCH_VAULT_KV_MOUNT` / `ORCH_VAULT_AUTH_MOUNT` | `kv` / `kubernetes` | Mount KV v2 và Kubernetes auth |

Token chỉ được điền vào ô mật khẩu; không in ra, không hiện trên trang. Vault
cho cụm kind local được chuẩn bị theo [runbook k8s-4f](../../orchestrator_docs/operations/k8s4f-dev.md#vault-riêng-cho-cụm-mới).
