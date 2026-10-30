---
id: RUNBOOK-STAGING-CATALOG
artifact: operations-runbook
status: current
last_reviewed: 2026-10-30
---

# Bootstrap Kubernetes staging catalog

Backend mặc định dùng `-seed-catalog staging`: chỉ tạo năm Types còn thiếu
(k8s-cluster, k8s-namespace, postgres, redis, workload), Organization và login
accounts theo profile. Không tạo Driver Accounts, Application, Environment,
Definitions hay Rendering template. Schema đã có không bị ghi đè. Restart hoặc
reapply giữ PostgreSQL PVC không xóa dữ liệu mẫu cũ. Không có reset/migration
hoặc cleanup tự động trong thay đổi này.

Fixture tests/development có thể chọn tường minh `-seed-catalog ''` (legacy),
`terraform-k8s`, hoặc profile `acceptance-v2` (catalog none). Chế độ staging
không đổi login-account policy: profile local vẫn có users fixture hiện hành.

## Bootstrap preset hiện hành

Để seed cả storage và workload catalog, dùng
[bootstrap tool](../../scripts/bootstrap/README.md):
`node scripts/bootstrap/run.mjs --target staging --url https://console-staging.example --kubeconfig /private/staging.kubeconfig`.
Preset staging có sẵn URL/user, Ceph class `rook-ceph-block`, Type/Definitions và
standard Git bundle với policy Recreate; dùng sẵn login seed mặc định, không
cần chép file password. ORCH_PLATFORM_PASSWORD_FILE chỉ dùng khi đổi mật khẩu. Account `k8s-4f` READY được reuse, chỉ inspect
kubeconfig một context khi chưa có account. Renderer mới scoped class `pvc`,
không thay default renderer/workload đang chạy. Chưa có app vẫn seed được;
không tạo app/deploy để kiểm tra. Phần bên dưới mô tả runner legacy chỉ đăng ký
installed catalog, không có storage/templates mới.

## Prepare

1. Deploy image mới. Container entrypoint bật installed renderer bằng
   `-score-k8s /usr/local/bin/score-k8s`; override đường dẫn bằng
   `ORCHESTRATOR_SCORE_K8S_PATH` nếu cần. Nếu gọi binary trực tiếp, truyền flag.
2. Đăng nhập bằng Platform Engineer/Admin. Trong **Infrastructure accounts**,
   đăng ký Internal account tên/key `k8s-4f`, upload kubeconfig thật và xác minh
   READY. Script không nhận hoặc tạo kubeconfig/account.
3. Backend phải có verified packaged modules `k8s-namespace`, `postgres-k8s`,
   `redis-k8s` và installed bundle `score-k8s-internal-v1` AVAILABLE.
   Nếu staging không có Internet, dùng image có provider mirror offline và
   [kiểm chứng image/patch staging](terraform-offline.md) trước Deploy.
   Plugin/module cache không thay thế provider mirror; giữ input namespace `name`.
4. Chuẩn bị file password đăng nhập ngoài repository, mode 0600 hoặc 0400.
   Không dùng kubeconfig hoặc Vault token làm password của Console.

## Run

Từ repository root, cài frontend dependencies và browser một lần:

```bash
cd frontend
npm ci
npx playwright install chromium
```

Từ thư mục `frontend`, chạy với browser hiển thị:

```bash
ORCH_PLATFORM_URL=https://devportal.stg.exampledevops.com \
ORCH_PLATFORM_USERNAME=platform-engineer \
ORCH_PLATFORM_PASSWORD_FILE=/private/path/console-password \
node test/e2e/bootstrap-staging-catalog.mjs
```

Có thể thêm `ORCH_HEADLESS=1` khi không có display. Script dùng locale Việt
trong browser context riêng, không sửa tùy chọn ngôn ngữ của browser cá nhân.
Không lưu session, password, trace, video hoặc screenshot. Script không nhận
password qua argument và không in raw browser/provider errors.

## Result

| ID | Purpose | Matching |
|---|---|---|
| staging-k8s-4f-cluster | existing-cluster, account k8s-4f | staging + class internal |
| staging-k8s-4f-namespace | verified Terraform k8s-namespace | staging |
| staging-k8s-4f-postgres | verified Terraform postgres-k8s | staging + class in-cluster |
| staging-k8s-4f-redis | verified Terraform redis-k8s | staging + class in-cluster |
| staging-k8s-4f-renderer | installed score-k8s template | staging |

Namespace, database/cache provider và renderer tham chiếu cùng descriptor
`k8s-cluster.internal#environments.@app.@env`. Namespace lấy tên từ
`context.env.namespace`; database/cache và renderer lấy namespace từ output
của `k8s-namespace.default#environments.@app.@env`. Không bind vào Application
mẫu. Workloads default class dùng template; PostgreSQL/Redis khai báo class
`in-cluster`. Các workloads trong cùng Environment muốn dùng chung database/
cache cần cùng resource ID; Environment khác có descriptor/scope riêng.

Script preflight trước mutation, mutations qua Console forms, GET metadata để
kiểm chứng. Rerun giữ cấu hình khớp; cấu hình khác, module ambiguity hoặc catalog
chồng lấn thì dừng, không update/delete. Khi fail có thể đã tạo một phần catalog;
kiểm tra Console rồi chạy lại. Không tự rollback các mục đã đăng ký thành công.

Script không tạo Application, secret values hoặc workload và không Deploy.
Sau đó người dùng tạo Application và chọn Secret Store. Vault phải unsealed,
Kubernetes auth đúng cluster; VSO/Agent và image registry pull phải được chuẩn
bị trước lượt deploy thật. Script không cài operator, DNS/TLS hay Gateway.
