---
id: RUNBOOK-ACCEPTANCE-MANUAL-K8S4F
artifact: operations-runbook
status: current
last_reviewed: 2026-10-09
related: UC-01, UC-04, UC-06, UC-08, UC-12, UC-16
---

# Triển khai acceptance app thủ công lên k8s-4f

Hướng dẫn dùng Web Console Docker tại `http://localhost:3001/ui/` và cụm kind
`kind-idp-internal` trên máy hiện tại. Ứng dụng gồm frontend, backend và PostgreSQL.
Không triển khai worker; job được submit sẽ ở trạng thái PENDING.

Connection `k8s-4f` đã được giữ lại ở trạng thái READY. Vault `vault-uc12` và VSO
đã có trên kind. Vault tạm của lượt E2E trước đã được xóa; muốn triển khai thủ công
cần chuẩn bị Secret Store cho workload theo các bước bên dưới.

[Bằng chứng E2E](../verification/2026-10-09-k8s4f-live.md) dùng backend test và
workload Vault riêng. Nó không chứng minh workload auth của Secret Store trong
Console lưu lâu dài đã sẵn sàng. Runbook này không tự chạy các lệnh triển khai.

## 1. Chuẩn bị Console và Connection

Từ root repository:

```bash
docker compose \
  -f docker-compose.yml \
  -f deploy/local/compose.kind.yml \
  up -d --build --wait
```

Overlay nối backend vào Docker network `kind` để truy cập node kind. Dùng overlay
này khi tạo lại container. Nếu đang dùng thêm `compose.source.yml`, tiếp tục giữ
file override đó trong lệnh Compose để giữ lựa chọn image hiện có.

Nếu đăng ký Connection từ đầu, xuất kubeconfig riêng chỉ chứa context của kind:

```bash
umask 077
kubectl --context kind-idp-internal config view \
  --minify --flatten --raw > /tmp/k8s-4f.kubeconfig

kubectl --kubeconfig /tmp/k8s-4f.kubeconfig config set-cluster \
  kind-idp-internal \
  --server=https://idp-internal-control-plane:6443
```

File chứa credential nhúng: không commit hoặc chia sẻ nội dung. Lệnh chỉ thay
endpoint trong bản sao, giữ CA/certificate và không đổi current context.

Mở `http://localhost:3001/ui/`, đăng nhập `platform-engineer` / `test-password`
(tài khoản seed local). Vào **Platform → Connections**:

1. Nhập Name `k8s-4f`.
2. Upload `/tmp/k8s-4f.kubeconfig`.
3. Bấm **Inspect kubeconfig**, chọn `kind-idp-internal`.
4. Bấm **Check and save**, xác nhận READY.

Connection đã tồn tại đúng cấu hình thì dùng lại. Không đăng ký Definition
`existing-cluster` riêng. Sau upload, xóa bản kubeconfig tạm khi không còn cần.

## 2. Chuẩn bị Vault và VSO

Kiểm tra các thành phần đã có:

```bash
kubectl --context kind-idp-internal -n vault get pods
kubectl --context kind-idp-internal -n vault exec vault-uc12-0 -- vault status
kubectl --context kind-idp-internal \
  -n vault-secrets-operator-system get pods
```

Vault phải unsealed và VSO phải Ready. Nếu thiếu thành phần, theo
[runbook Vault trên kind](vault-kind.md); không cài đè các release hiện có.

Mở đường truy cập Vault cho cả trình duyệt và backend Docker; giữ terminal này chạy:

```bash
kubectl --context kind-idp-internal -n vault \
  port-forward --address 127.0.0.1,172.18.0.1 \
  svc/vault-uc12 18200:8200
```

`172.18.0.1` là gateway IPv4 của Docker network `kind` trên máy đã đối chiếu.
Nếu network khác, kiểm tra gateway bằng `docker network inspect kind` và thay
địa chỉ bind/Backend address tương ứng.

| Nơi truy cập | Địa chỉ |
|---|---|
| Trình duyệt/Vault CLI | `http://127.0.0.1:18200` |
| Backend Docker | `http://172.18.0.1:18200` |
| Pod/VSO trong kind | `http://vault-uc12.vault.svc:8200` |

Dùng tài khoản quản trị Vault để kiểm tra:

- Secrets engine KV v2 tại mount `kv`.
- Kubernetes auth tại mount `kubernetes`.
- Auth cấu hình đúng Kubernetes API/CA của cụm kind.
- ServiceAccount reviewer có quyền `system:auth-delegator`.

Các thành phần này đã được cấu hình trước trên `vault-uc12`; xác nhận cấu hình
hiện tại trước khi thay đổi. Nếu thiết lập lại, Vault chạy trong Kubernetes có
thể dùng ServiceAccount token của chính Pod làm reviewer khi ServiceAccount có
quyền TokenReview. Tham khảo
[Kubernetes auth của Vault](https://developer.hashicorp.com/vault/docs/auth/kubernetes).

### Tạo policy/token scoped cho Orchestrator

Policy phù hợp có trong [applications-policy.hcl](../../deploy/local/applications-policy.hcl).
Với Vault CLI đã đăng nhập bằng tài khoản quản trị, từ root repository:

```bash
export VAULT_ADDR=http://127.0.0.1:18200

vault policy write orch-acceptance-backend \
  deploy/local/applications-policy.hcl
```

Tạo token gắn policy `orch-acceptance-backend` qua giao diện Vault hoặc
`vault token create`, lưu riêng và dùng ở bước 3. Token cung cấp cho Orchestrator
là token scoped, không phải token quản trị. Quản lý TTL/renewal theo thời gian
muốn giữ ứng dụng.

Không mặc định dùng lại token backend cũ: lượt test trước đã xác nhận token đó
bị 403 khi đọc `auth/kubernetes/config`. Policy trên có quyền đọc đường dẫn này,
cùng quyền đọc/ghi application values, quản lý workload policy và auth role.

## 3. Đăng ký Secret Store trong Console

Bằng tài khoản `platform-engineer`, vào **Platform → Secret stores**:

| Trường | Giá trị |
|---|---|
| Name | `Kind Vault` |
| Backend address | `http://172.18.0.1:18200` |
| Workload address | `http://vault-uc12.vault.svc:8200` |
| KV v2 mount | `kv` |
| Kubernetes auth mount | `kubernetes` |
| Token | Token scoped vừa tạo |

Đăng ký và xác nhận READY, Kubernetes auth CONFIGURED. CONFIGURED chứng minh
backend đọc được cấu hình auth; kiểm tra workload/VSO thật vẫn diễn ra khi Deploy.

Store này khác `Platform Vault` của Docker Compose. Chọn **Kind Vault** để cấp
secret cho workload trên kind. Nếu store matching đã được đăng ký, dùng lại.

## 4. Build và load image acceptance app

```bash
cd /home/thanhnt1/projects/poc/backend

bash test/integration/build-images.sh manual

kind load docker-image \
  acceptance-backend:manual \
  acceptance-frontend:manual \
  --name idp-internal
```

Script build thêm worker, nhưng hướng dẫn này chỉ triển khai FE và BE.
PostgreSQL được tạo từ resource của BE. Image được load trực tiếp vào kind nên
không cần push registry cho lần triển khai local này.

## 5. Tạo Application và cấu hình staging

Đăng nhập `developer` / `test-password`.

Tạo Application, ví dụ Name/Subdomain `acceptance-manual`. Trong **staging → Settings**:

1. Execution Connection: `k8s-4f` → Save.
2. Secret Store: **Kind Vault** → Save.

Thêm các cấu hình và lưu pending change:

| Loại | Key | Value |
|---|---|---|
| Variable | `ACCEPTANCE_CONFIG` | `acceptance-config-ok` |
| Secret | `ACCEPTANCE_SECRET` | Chuỗi thử nghiệm tự chọn |
| Variable | `ACCEPTANCE_SECRET_SHA256` | SHA-256 dạng hex của đúng chuỗi secret trên |

Tính SHA-256 không thêm newline vào chuỗi. Secret và hash phải khớp thì kiểm tra
secret mới PASS. Chỉ staging được triển khai; production chưa cấu hình thì để nguyên.

## 6. Khai báo backend và frontend

Trong **+ Add workload → Enter on form**, tạo backend:

| Trường | Giá trị |
|---|---|
| Workload name | `backend` |
| Container name | `main` |
| Image | `acceptance-backend:manual` |
| Service port name | `http` |
| Service port / target port | `8080` / `8080` |

Thêm resource:

| Trường | Giá trị |
|---|---|
| Alias | `db` |
| Type / Class | `postgres` / `default` |
| Database / Username | `acceptance` / `acceptance` |

Trong container, tick ba Application keys đã tạo, dùng cùng tên biến container.
Thêm các binding **Other sources → Resource output**:

| Biến container | Resource/output |
|---|---|
| `PGHOST` | `db.host` |
| `PGPORT` | `db.port` |
| `PGDATABASE` | `db.database` |
| `PGUSER` | `db.username` |
| `PGPASSWORD` | `db.password` — secret |

Bấm **Save pending workload**.

Tiếp theo tạo frontend:

| Trường | Giá trị |
|---|---|
| Workload name | `frontend` |
| Container name | `main` |
| Image | `acceptance-frontend:manual` |
| Service port name | `http` |
| Service port / target port | `8080` / `8080` |
| `BACKEND_URL` | Other sources → Workload Service → `backend` → `http` |

Bấm **Save pending workload**. Port-forward ở bước 7 đủ cho truy cập local;
không cần thêm public path/Ingress để thực hiện kiểm tra này.

Catalog cần Definition namespace và PostgreSQL phù hợp `internal-k8s`; seed hiện
có `namespace-kubernetes` và `postgres-internal-statefulset`. Cluster node được
tạo ngầm từ Environment Connection theo ADR-013.

## 7. Preview, Deploy và kiểm tra

Trong Application:

1. Bấm **Preview changes**.
2. Kiểm tra target staging, Connection `k8s-4f`, hai workload bị ảnh hưởng.
3. Bấm **Deploy these changes**.
4. Chờ backend/frontend báo succeeded.

Namespace có dạng `app-<application-id>-staging`. Thay placeholder bằng ID từ
URL Application trước khi chạy:

```bash
kubectl --context kind-idp-internal \
  -n app-<application-id>-staging \
  get deployment,statefulset,pod,pvc
```

FE/BE và PostgreSQL phải Ready, PVC phải Bound. Mở frontend bằng port-forward:

```bash
kubectl --context kind-idp-internal \
  -n app-<application-id>-staging \
  port-forward svc/frontend 18080:8080
```

Truy cập `http://localhost:18080`. Bốn dòng phải PASS:

- backend connection
- environment
- secret
- database

Submit job để kiểm tra backend ghi nhận job. Job ở trạng thái PENDING vì chưa
triển khai worker; hướng dẫn không kiểm chứng worker processing.

Ứng dụng tiếp tục chạy sau khi đóng frontend port-forward. Vault port-forward
ở bước 2 cần duy trì để backend Console truy cập Secret Store; Pod/VSO dùng địa
chỉ Service bên trong cluster. Lệnh thủ công không tự cleanup namespace, workload
hoặc dữ liệu PostgreSQL. Khi muốn giữ ứng dụng, giữ các resource này và theo dõi
token Vault/TTL cùng trạng thái unseal sau restart.
