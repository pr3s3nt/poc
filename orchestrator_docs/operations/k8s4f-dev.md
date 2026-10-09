---
id: RUNBOOK-K8S4F-DEV
artifact: operations-runbook
status: current
last_reviewed: 2026-10-09
---

# Môi trường dev Docker IDP và cụm K8S-4F

IDP chạy trên máy bằng Docker Compose. Ứng dụng mẫu frontend/backend/PostgreSQL
chạy trên cụm kind **mới** `k8s-4f`, context `kind-k8s-4f`.
Connection hiển thị **K8S-4F**, key `k8s-4f-2`; Connection `k8s-4f` cũ vẫn
trỏ tới `kind-idp-internal`. Khi chọn đích, dùng tên hiển thị và key mới.

## Truy cập và khởi động IDP

```bash
docker compose -f docker-compose.yml -f deploy/local/compose.kind.yml up -d --wait
docker compose ps
kubectl --context kind-k8s-4f get nodes
```

Overlay giữ backend trên network `kind` để endpoint
`https://k8s-4f-control-plane:6443` của Connection truy cập được từ Docker.
Network và cụm đã có; không chạy lại `kind create cluster` khi cụm còn tồn tại.
Console: <http://localhost:3001/ui/>. Tài khoản local `platform-engineer` và
`developer` dùng password `test-password` theo [Compose runbook](docker-local.md).

Application bàn giao **k8s4f-dev-final** có ID `3c5e4c33-b724-4032-80b1-d270e5c32a35`.
Staging chọn Connection `k8s-4f-2` và Secret Store **K8S-4F Vault**
(`k8s-4f-vault`). Production chưa triển khai.

## Workload và dữ liệu giữ lại

Namespace: `app-3c5e4c33-b724-4032-80b1-d270e5c32a35-staging`.
Image FE/BE: `acceptance-frontend:k8s4f-dev` và `acceptance-backend:k8s4f-dev`,
được load vào cụm mới. PostgreSQL là resource `db` của backend, có PVC 1 GiB.
Không có worker; job được nhận ở trạng thái PENDING.

```bash
kubectl --context kind-k8s-4f \
  -n app-3c5e4c33-b724-4032-80b1-d270e5c32a35-staging \
  get deployment,statefulset,pod,pvc
```

Frontend truy cập qua <http://localhost:18480/> khi port-forward chạy. Để mở
lại đường truy cập sau khi khởi động máy:

```bash
kubectl --context kind-k8s-4f \
  -n app-3c5e4c33-b724-4032-80b1-d270e5c32a35-staging \
  port-forward svc/frontend 18480:8080
```

Giữ terminal chạy; nếu port đang được supervisor của lượt triển khai sử dụng,
dùng URL có sẵn hoặc chọn port khác. Đóng port-forward không xóa workload.

Application từ lượt quay trước **k8s4f-dev** cũng được giữ lại, ID
`a43bb834-d2d7-42cd-a035-41841155b5f7`, namespace
`app-a43bb834-d2d7-42cd-a035-41841155b5f7-staging`. URL port 18480 được bàn giao
cho bản **k8s4f-dev-final**; bản trước không bị xóa dữ liệu.

## Vault riêng cho cụm mới

Container `k8s-4f-workload-vault` nằm trên Docker network `kind`, dùng volume
riêng cho data/bootstrap, HTTP nội bộ `http://k8s-4f-workload-vault:8200`.
Backend và VSO dùng địa chỉ này; không cần Vault port-forward.
VSO chạy trong namespace `vault-secrets-operator-system` trên cụm mới.

Script [start.sh](../../deploy/local/workload-vault/start.sh) chuẩn bị Vault và
Kubernetes auth cho đúng cụm. Xem ownership preflight của script trước khi chạy
lại. Scoped token nằm ngoài Git trong
`~/.local/share/poc-k8s-4f-vault/token`; không hiển thị hoặc chia sẻ nội dung.
Root/unseal material giữ trong bootstrap volume, tách khỏi backend IDP.

Vault được đặt restart policy `unless-stopped`, unseal tự động và renew token.
Token có period 768 giờ; nếu hết hạn hoặc bị revoke và script tạo token mới,
credential đã lưu trong Secret Store không tự đổi theo file. Cần xử lý đăng ký
store/credential hợp lệ theo khả năng Console trước khi Deploy tiếp.

Giữ PostgreSQL/PVC, Vault data/bootstrap và các volume Compose. Xóa cluster,
PVC hoặc volume có thể mất dữ liệu; đây là môi trường dev trên một máy,
không thay thế backup.
